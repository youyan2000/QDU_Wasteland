package githubdigest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newFakeGitHub(t *testing.T, responses map[string][]map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/repositories" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query().Get("q")
		for key, items := range responses {
			if strings.Contains(q, key) {
				_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
				return
			}
		}
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
}

func repo(full, name, desc string, stars int, topics []string) map[string]any {
	return map[string]any{
		"full_name":        full,
		"name":             name,
		"html_url":         "https://github.com/" + full,
		"description":      desc,
		"stargazers_count": stars,
		"forks_count":      stars/50 + 1,
		"language":         "C++",
		"topics":           topics,
		"pushed_at":        time.Now().UTC().Format(time.RFC3339),
		"owner":            map[string]any{"avatar_url": "https://avatars.example/" + full + ".png"},
	}
}

func TestRefreshDeduplicatesAndGroups(t *testing.T) {
	srv := newFakeGitHub(t, map[string][]map[string]any{
		"topic:kicad": {
			repo("acme/board", "board", "KiCad PCB templates", 1200, []string{"kicad", "pcb"}),
			repo("acme/shared", "shared", "shared schematic library", 900, []string{"kicad"}),
		},
		"topic:pcb": {
			// acme/shared repeats: must be de-duplicated.
			repo("acme/shared", "shared", "shared schematic library", 900, []string{"pcb"}),
			repo("acme/plotter", "plotter", "Gerber plotter", 300, []string{"pcb"}),
		},
		"topic:llm": {
			repo("acme/chat", "chat", "A local chat interface for models", 15000, []string{"llm"}),
		},
	})
	defer srv.Close()

	store := NewStore(t.TempDir(), "")
	store.APIBase = srv.URL
	store.Client = srv.Client()
	store.Delay = 0
	store.RetryDelay = 0
	store.Queries = []Query{
		{Name: "kicad", Group: GroupHardware, Kind: "active", Q: "topic:kicad", PerPage: 5, Sort: "stars"},
		{Name: "pcb", Group: GroupHardware, Kind: "active", Q: "topic:pcb", PerPage: 5, Sort: "stars"},
		{Name: "llm", Group: GroupGeneral, Kind: "active", Q: "topic:llm", PerPage: 5, Sort: "stars"},
	}

	d, err := store.Refresh(context.Background(), true)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if d.Date != today() {
		t.Fatalf("date = %q", d.Date)
	}
	if len(d.Items) != 4 {
		t.Fatalf("items = %d, want 4: %+v", len(d.Items), d.Items)
	}
	var hardware, general int
	for _, it := range d.Items {
		switch it.Group {
		case GroupHardware:
			hardware++
			if !isHardwareRelevant(it) {
				t.Errorf("non-hardware item in hardware group: %s", it.FullName)
			}
		case GroupGeneral:
			general++
			if isHardwareRelevant(it) {
				t.Errorf("hardware item leaked into general group: %s", it.FullName)
			}
		default:
			t.Errorf("missing group on %s", it.FullName)
		}
	}
	if hardware != 3 || general != 1 {
		t.Fatalf("groups hardware=%d general=%d, want 3 and 1", hardware, general)
	}
	// Hardware section comes first, and the strongest match leads.
	if d.Items[0].Group != GroupHardware || d.Items[0].FullName != "acme/board" {
		t.Fatalf("first item = %+v", d.Items[0])
	}

	latest, err := store.Latest()
	if err != nil || latest == nil || len(latest.Items) != 4 {
		t.Fatalf("latest: %v %+v", err, latest)
	}
	day, err := store.Get(d.Date)
	if err != nil || len(day.Items) != 4 {
		t.Fatalf("get day: %v", err)
	}
	days, err := store.Days()
	if err != nil || len(days) != 1 || days[0] != d.Date {
		t.Fatalf("days: %v %v", err, days)
	}
}

func TestGetRejectsPathTraversal(t *testing.T) {
	store := NewStore(t.TempDir(), "")
	for _, bad := range []string{"", "../latest", "2026-1-1", "2026-01-01/../../etc/passwd"} {
		if _, err := store.Get(bad); err == nil {
			t.Fatalf("Get(%q) should fail", bad)
		}
	}
}

func TestRefreshFailsWhenNothingReturned(t *testing.T) {
	srv := newFakeGitHub(t, map[string][]map[string]any{})
	defer srv.Close()
	store := NewStore(t.TempDir(), "")
	store.APIBase = srv.URL
	store.Client = srv.Client()
	store.Delay = 0
	store.RetryDelay = 0
	store.Queries = []Query{{Name: "kicad", Group: GroupHardware, Kind: "active", Q: "topic:kicad", PerPage: 3, Sort: "stars"}}
	if _, err := store.Refresh(context.Background(), true); err == nil {
		t.Fatal("expected error when GitHub returns no repositories")
	}
}

func TestIsHardwareRelevant(t *testing.T) {
	cases := []struct {
		name string
		item Item
		want bool
	}{
		{"kicad project", Item{FullName: "a/board", Description: "KiCad PCB templates"}, true},
		{"fpga core", Item{FullName: "a/core", Description: "RISC-V soft core for FPGA"}, true},
		{"firmware", Item{FullName: "a/fw", Description: "Alternative firmware for ESP8266"}, true},
		{"agent false positive", Item{FullName: "a/harness", Description: "The ultimate harness for coding agents. Start with code, then CAD, circuits, robots"}, false},
		{"plain software", Item{FullName: "a/web", Description: "A tiny web framework"}, false},
		{"generic repo", Item{FullName: "a/shared", Description: "shared repo"}, false},
	}
	for _, c := range cases {
		if got := isHardwareRelevant(c.item); got != c.want {
			t.Errorf("%s: isHardwareRelevant = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPlausiblePopularity(t *testing.T) {
	if !plausiblePopularity(Item{Stars: 1200}) {
		t.Error("small project should pass without fork data")
	}
	if !plausiblePopularity(Item{Stars: 50000, Forks: 5000}) {
		t.Error("healthy star/fork ratio should pass")
	}
	if plausiblePopularity(Item{Stars: 350000, Forks: 10}) {
		t.Error("implausible star/fork ratio should be rejected")
	}
}

func TestPickFreshPrefersUnseen(t *testing.T) {
	list := []Item{{FullName: "a/old"}, {FullName: "a/new"}}
	got := pickFresh(list, 2, map[string]int{"a/old": 1})
	if len(got) != 2 || got[0].FullName != "a/new" {
		t.Fatalf("pickFresh = %+v", got)
	}
	// When everything was seen, it still fills the quota.
	got = pickFresh(list, 2, map[string]int{"a/old": 3, "a/new": 1})
	if len(got) != 2 {
		t.Fatalf("pickFresh with all seen = %+v", got)
	}
}

func TestPickFreshCyclesStalestFirst(t *testing.T) {
	list := []Item{{FullName: "a/recent"}, {FullName: "a/middle"}, {FullName: "a/ancient"}}
	seen := map[string]int{"a/recent": 1, "a/middle": 4, "a/ancient": 9}
	got := pickFresh(list, 2, seen)
	if len(got) != 2 {
		t.Fatalf("pickFresh = %+v", got)
	}
	if got[0].FullName != "a/ancient" || got[1].FullName != "a/middle" {
		t.Fatalf("expected stalest first, got %+v", got)
	}
}

// TestRefreshVariesFromPreviousDay pins the promise that the daily digest does
// not repeat yesterday's repositories while unseen candidates remain.
func TestRefreshVariesFromPreviousDay(t *testing.T) {
	// The digest size is 16, so the pool must hold more than two days of
	// distinct candidates for a no-repeat guarantee to be testable.
	items := make([]map[string]any, 0, 48)
	for i := 1; i <= 48; i++ {
		name := fmt.Sprintf("board-%02d", i)
		items = append(items, repo("acme/"+name, name, "KiCad PCB library", 500+i*10, []string{"kicad", "pcb"}))
	}
	srv := newFakeGitHub(t, map[string][]map[string]any{"topic:kicad": items})
	defer srv.Close()

	store := NewStore(t.TempDir(), "")
	store.APIBase = srv.URL
	store.Client = srv.Client()
	store.Delay = 0
	store.RetryDelay = 0
	store.Queries = []Query{{Name: "kicad", Group: GroupHardware, Kind: "active", Q: "topic:kicad", PerPage: 48, Sort: "stars"}}

	day1, err := store.Refresh(context.Background(), true)
	if err != nil {
		t.Fatalf("refresh day1: %v", err)
	}
	if len(day1.Items) < 8 {
		t.Fatalf("day1 only produced %d items", len(day1.Items))
	}

	prev := &Digest{
		Date:        time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
		GeneratedAt: time.Now().Format(time.RFC3339),
		Items:       day1.Items,
	}
	if err := store.save(prev); err != nil {
		t.Fatalf("save previous day: %v", err)
	}

	day2, err := store.Refresh(context.Background(), true)
	if err != nil {
		t.Fatalf("refresh day2: %v", err)
	}
	seen := make(map[string]bool, len(day1.Items))
	for _, it := range day1.Items {
		seen[it.FullName] = true
	}
	for _, it := range day2.Items {
		if seen[it.FullName] {
			t.Fatalf("%s repeated from the previous day", it.FullName)
		}
	}
	if len(day2.Items) != len(day1.Items) {
		t.Fatalf("day2 items = %d, want %d", len(day2.Items), len(day1.Items))
	}
}

// TestRefreshBuildsFourSections pins the themed layout: hardware, AI, music
// and a catch-all popular section, each filled to its quota.
func TestRefreshBuildsFourSections(t *testing.T) {
	hw := make([]map[string]any, 0, 10)
	for i := 1; i <= 10; i++ {
		name := fmt.Sprintf("board-%02d", i)
		hw = append(hw, repo("acme/"+name, name, "KiCad PCB library", 900+i*10, []string{"kicad"}))
	}
	ai := make([]map[string]any, 0, 8)
	for i := 1; i <= 8; i++ {
		name := fmt.Sprintf("model-%02d", i)
		ai = append(ai, repo("lab/"+name, name, "Large language model training toolkit", 4000+i*10, []string{"llm"}))
	}
	music := make([]map[string]any, 0, 5)
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("synth-%02d", i)
		music = append(music, repo("band/"+name, name, "Browser based music sequencer", 800+i*10, []string{"music"}))
	}
	general := make([]map[string]any, 0, 8)
	for i := 1; i <= 8; i++ {
		name := fmt.Sprintf("app-%02d", i)
		general = append(general, repo("shop/"+name, name, "Note taking app for teams", 3000+i*10, []string{"productivity"}))
	}
	srv := newFakeGitHub(t, map[string][]map[string]any{
		"topic:kicad":    hw,
		"topic:llm":      ai,
		"topic:music":    music,
		"topic:frontend": general,
	})
	defer srv.Close()

	store := NewStore(t.TempDir(), "")
	store.APIBase = srv.URL
	store.Client = srv.Client()
	store.Delay = 0
	store.RetryDelay = 0
	store.Queries = []Query{
		{Name: "kicad", Group: GroupHardware, Kind: "active", Q: "topic:kicad", PerPage: 10, Sort: "stars"},
		{Name: "llm", Group: GroupAI, Kind: "active", Q: "topic:llm", PerPage: 8, Sort: "stars"},
		{Name: "music", Group: GroupMusic, Kind: "active", Q: "topic:music", PerPage: 5, Sort: "stars"},
		{Name: "other", Group: GroupGeneral, Kind: "active", Q: "topic:frontend", PerPage: 8, Sort: "stars"},
	}

	d, err := store.Refresh(context.Background(), true)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	counts := map[string]int{}
	for _, it := range d.Items {
		counts[it.Group]++
	}
	want := map[string]int{GroupHardware: 6, GroupAI: 4, GroupMusic: 2, GroupGeneral: 4}
	for _, g := range GroupOrder {
		if counts[g] != want[g] {
			t.Fatalf("%s section = %d items, want %d (all: %v)", g, counts[g], want[g], counts)
		}
	}
	if len(d.Items) != 16 {
		t.Fatalf("total items = %d, want 16", len(d.Items))
	}
	// Sections must be stored hardware-first so the page renders in order.
	var order []string
	seenGroup := map[string]bool{}
	for _, it := range d.Items {
		if seenGroup[it.Group] {
			continue
		}
		seenGroup[it.Group] = true
		order = append(order, it.Group)
	}
	for i, g := range GroupOrder {
		if order[i] != g {
			t.Fatalf("section order = %v, want %v", order, GroupOrder)
		}
	}
}
