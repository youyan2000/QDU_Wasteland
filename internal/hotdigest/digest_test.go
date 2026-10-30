package hotdigest

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testNow() time.Time {
	return time.Date(2026, 10, 30, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
}

type feedItem struct {
	title     string
	link      string
	summary   string
	published time.Time
}

func rssDoc(items ...feedItem) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>t</title>`)
	for _, it := range items {
		b.WriteString("<item><title>")
		b.WriteString(html.EscapeString(it.title))
		b.WriteString("</title><link>")
		b.WriteString(html.EscapeString(it.link))
		b.WriteString("</link>")
		if it.summary != "" {
			b.WriteString("<description>")
			b.WriteString(html.EscapeString(it.summary))
			b.WriteString("</description>")
		}
		if !it.published.IsZero() {
			b.WriteString("<pubDate>")
			b.WriteString(it.published.Format(time.RFC1123Z))
			b.WriteString("</pubDate>")
		}
		b.WriteString("</item>")
	}
	b.WriteString("</channel></rss>")
	return b.String()
}

// newFeedServer serves one named feed document per path and counts requests.
func newFeedServer(t *testing.T, feeds map[string]string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		body, ok := feeds[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestRefreshBuildsAllSixSections(t *testing.T) {
	now := testNow()
	srv, _ := newFeedServer(t, map[string]string{
		"essay.xml": rssDoc(
			feedItem{"On the uses of silence", "https://essay.example/silence", "An essay on listening.", now.Add(-2 * time.Hour)},
			feedItem{"A thirty day old essay", "https://essay.example/old", "Stale.", now.Add(-40 * 24 * time.Hour)},
		),
		"power.xml": rssDoc(
			feedItem{"SiC inverter cuts switching losses", "https://power.example/sic", "A wide bandgap design.", now.Add(-4 * time.Hour)},
			feedItem{"Quarterly earnings call transcript", "https://power.example/earnings", "Revenue up.", now.Add(-1 * time.Hour)},
			feedItem{"Sponsored: buy our new probe", "https://power.example/ad", "", now.Add(-1 * time.Hour)},
		),
		"embedded.xml": rssDoc(
			feedItem{"RP2040 board ships with Rust firmware support", "https://emb.example/rp2040", "", now.Add(-5 * time.Hour)},
		),
		"ai-wire.xml": rssDoc(
			feedItem{"Open-weight model beats coding benchmarks", "https://ai-wire.example/model", "Released today.", now.Add(-2 * time.Hour)},
		),
		"ai-desk.xml": rssDoc(
			feedItem{"Researchers say open weight model beats coding benchmarks", "https://ai-desk.example/story", "", now.Add(-3 * time.Hour)},
		),
		"music.xml": rssDoc(
			feedItem{"A new album of modular synth improvisations", "https://music.example/album", "", now.Add(-3 * time.Hour)},
		),
		"poetry.xml": rssDoc(
			feedItem{"A poem about the long walk home", "https://poetry.example/walk", "", now.Add(-6 * time.Hour)},
		),
	})
	store := NewStore(t.TempDir())
	store.Now = func() time.Time { return now }
	store.Sources = []Source{
		{ID: "essay", Name: "Essay Weekly", Group: GroupEssay, Track: TrackEn, Tier: "T1_5", Lang: "en", Feed: srv.URL + "/essay.xml"},
		{ID: "poetry", Name: "Poetry Wire", Group: GroupPoetry, Track: TrackEn, Tier: "T1", Lang: "en", Feed: srv.URL + "/poetry.xml"},
		{ID: "power", Name: "Power Wire", Group: GroupPower, Tier: "T2", Lang: "en", Broad: true, Feed: srv.URL + "/power.xml"},
		{ID: "embedded", Name: "Embedded Wire", Group: GroupEmbedded, Tier: "T1_5", Lang: "en", Feed: srv.URL + "/embedded.xml"},
		{ID: "ai-wire", Name: "AI Wire", Group: GroupAI, Tier: "T1", Lang: "en", Feed: srv.URL + "/ai-wire.xml"},
		{ID: "ai-desk", Name: "AI Desk", Group: GroupAI, Tier: "T2", Lang: "en", Feed: srv.URL + "/ai-desk.xml"},
		{ID: "music", Name: "Music Wire", Group: GroupMusic, Tier: "T2", Lang: "en", Feed: srv.URL + "/music.xml"},
	}

	d, err := store.Refresh(context.Background(), true)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	byGroup := make(map[string][]Item)
	for _, it := range d.Items {
		byGroup[it.Group] = append(byGroup[it.Group], it)
	}
	for _, group := range []string{GroupEssay, GroupPoetry, GroupPower, GroupEmbedded, GroupAI, GroupMusic} {
		if len(byGroup[group]) == 0 {
			t.Errorf("section %q is empty", group)
		}
	}
	// 哲学理论 only has journal sources, so it stays empty in this fixture.
	if len(byGroup[GroupPhilosophy]) != 0 {
		t.Errorf("philosophy section should be empty without journal sources, got %d", len(byGroup[GroupPhilosophy]))
	}
	// Split fields carry their language track on the item.
	for _, it := range byGroup[GroupEssay] {
		if it.Track != TrackEn {
			t.Errorf("essay item track = %q, want %q", it.Track, TrackEn)
		}
	}

	if got := len(byGroup[GroupEssay]); got != 1 {
		t.Errorf("essay items = %d, want 1 (the 40 day old essay is outside the 30 day window)", got)
	}
	for _, it := range d.Items {
		if strings.Contains(it.Title, "Sponsored") {
			t.Error("marketing copy was included")
		}
		if strings.Contains(it.Title, "earnings") {
			t.Error("an off-topic item from a broad feed was included")
		}
	}

	ai := byGroup[GroupAI]
	if len(ai) != 1 {
		t.Fatalf("two reports of one story should cluster, got %d", len(ai))
	}
	if ai[0].Source != "AI Wire" || ai[0].Tier != "T1" {
		t.Errorf("representative = %q (%s), want the first-party source", ai[0].Source, ai[0].Tier)
	}
	if ai[0].SourceCount != 2 {
		t.Errorf("coverage = %d, want 2", ai[0].SourceCount)
	}
	if d.SourceTotal != 7 || d.SourceFailed != 0 {
		t.Errorf("source totals = %d/%d", d.SourceTotal, d.SourceFailed)
	}
}

func TestCapBySourceLimitsCuratedFlood(t *testing.T) {
	items := make([]Item, 0, 25)
	for i := 0; i < 20; i++ {
		items = append(items, mkItem(GroupPhilosophy, "one-journal", KindPaper, fmt.Sprintf("p%02d", i), float64(100-i)))
	}
	for i := 0; i < 5; i++ {
		items = append(items, mkItem(GroupPhilosophy, "other", KindPaper, fmt.Sprintf("o%02d", i), float64(50-i)))
	}
	got := capBySource(items, groupSpecs[GroupPhilosophy])
	per := map[string]int{}
	for _, it := range got {
		per[it.SourceID]++
	}
	if per["one-journal"] > groupSpecs[GroupPhilosophy].perSource {
		t.Errorf("one journal kept %d items, cap is %d", per["one-journal"], groupSpecs[GroupPhilosophy].perSource)
	}
	// Both sources are capped, so the total is the cap twice.
	if len(got) != groupSpecs[GroupPhilosophy].perSource*2 {
		t.Errorf("items = %d, want %d", len(got), groupSpecs[GroupPhilosophy].perSource*2)
	}
}

func TestSectionsSplitByTrack(t *testing.T) {
	lists := sections()
	if len(lists) != 13 {
		t.Fatalf("sections = %d, want 13 (3 split fields × 3 tracks + 4 whole fields)", len(lists))
	}
	keys := make(map[string]bool, len(lists))
	for _, s := range lists {
		keys[s.Key] = true
	}
	for _, want := range []string{
		"phil/zh", "phil/en", "phil/world",
		"essay/zh", "essay/en", "essay/world",
		"poetry/zh", "poetry/en", "poetry/world",
		"power", "embedded", "ai", "music",
	} {
		if !keys[want] {
			t.Errorf("missing section %q", want)
		}
	}
	for _, group := range []string{GroupPhilosophy, GroupEssay, GroupPoetry} {
		if !IsSplit(group) {
			t.Errorf("%q should be split by language", group)
		}
	}
	if IsSplit(GroupPower) {
		t.Error("power electronics is not a split field")
	}
}

func sortedPool(items []Item) []Item {
	sortItems(items)
	return items
}

func mkItem(group, sourceID, kind, id string, heat float64) Item {
	return Item{
		ID: id, URL: "https://example.com/" + group + "/" + id, Group: group,
		SourceID: sourceID, Source: sourceID, Kind: kind, Heat: heat,
		Title: "story " + id,
	}
}

func TestSelectGroupReachesTarget(t *testing.T) {
	pool := make([]Item, 0, 60)
	for i := 0; i < 60; i++ {
		pool = append(pool, mkItem(GroupAI, fmt.Sprintf("s%d", i%20), KindNews, fmt.Sprintf("n%02d", i), float64(100-i)))
	}
	got := selectGroup(sortedPool(pool), groupSpecs[GroupAI], nil)
	if len(got) != groupSpecs[GroupAI].target {
		t.Fatalf("items = %d, want %d", len(got), groupSpecs[GroupAI].target)
	}
}

func TestSelectGroupRespectsPerSourceCap(t *testing.T) {
	pool := []Item{}
	for i := 0; i < 20; i++ {
		pool = append(pool, mkItem(GroupAI, "prolific", KindNews, fmt.Sprintf("p%02d", i), float64(100-i)))
	}
	for i := 0; i < 5; i++ {
		pool = append(pool, mkItem(GroupAI, "other", KindNews, fmt.Sprintf("o%02d", i), float64(50-i)))
	}
	got := selectGroup(sortedPool(pool), groupSpecs[GroupAI], nil)
	perSource := map[string]int{}
	for _, it := range got {
		perSource[it.SourceID]++
	}
	if perSource["prolific"] > groupSpecs[GroupAI].perSource {
		t.Errorf("prolific contributed %d, cap is %d", perSource["prolific"], groupSpecs[GroupAI].perSource)
	}
}

func TestSelectGroupNeverFallsBelowAvailable(t *testing.T) {
	pool := []Item{
		mkItem(GroupAI, "a", KindNews, "a1", 9),
		mkItem(GroupAI, "b", KindNews, "b1", 8),
	}
	got := selectGroup(sortedPool(pool), groupSpecs[GroupAI], nil)
	if len(got) != 2 {
		t.Fatalf("items = %d, want both candidates (a thin pool is never padded)", len(got))
	}
}

func TestSelectGroupSkipsRecentlyShownWhenPoolIsWide(t *testing.T) {
	pool := []Item{mkItem(GroupAI, "seen", KindNews, "seen", 999)}
	recent := map[string]int{"seen": 1}
	for i := 0; i < 40; i++ {
		pool = append(pool, mkItem(GroupAI, fmt.Sprintf("s%d", i%10), KindNews, fmt.Sprintf("n%02d", i), float64(100-i)))
	}
	got := selectGroup(sortedPool(pool), groupSpecs[GroupAI], recent)
	for _, it := range got {
		if it.ID == "seen" {
			t.Fatal("a story shown in a recent issue was included")
		}
	}
	if len(got) != groupSpecs[GroupAI].target {
		t.Errorf("items = %d, want %d", len(got), groupSpecs[GroupAI].target)
	}
}

func TestSelectGroupMixesPapersAndNews(t *testing.T) {
	pool := []Item{}
	// Fresh papers dominate on heat; news must still keep its reserved share.
	// Papers come from several journals and news from several outlets, as in a
	// real section, so the per-source cap does not decide the outcome.
	for i := 0; i < 40; i++ {
		pool = append(pool, mkItem(GroupPower, fmt.Sprintf("journal%d", i%5), KindPaper, fmt.Sprintf("p%02d", i), float64(200-i)))
	}
	for i := 0; i < 20; i++ {
		pool = append(pool, mkItem(GroupPower, fmt.Sprintf("news%d", i%8), KindNews, fmt.Sprintf("n%02d", i), float64(50-i)))
	}
	got := selectGroup(sortedPool(pool), groupSpecs[GroupPower], nil)
	papers, news := 0, 0
	for _, it := range got {
		if it.Kind == KindPaper {
			papers++
		} else {
			news++
		}
	}
	if len(got) != groupSpecs[GroupPower].target {
		t.Fatalf("items = %d, want %d", len(got), groupSpecs[GroupPower].target)
	}
	if news == 0 {
		t.Fatalf("papers crowded out all press coverage (%d papers)", papers)
	}
	if float64(news) < float64(len(got))*0.25 {
		t.Errorf("news share = %d/%d, want at least a quarter", news, len(got))
	}
}

func TestRefreshRecordsFailedSources(t *testing.T) {
	now := testNow()
	srv, _ := newFeedServer(t, map[string]string{
		"ok.xml": rssDoc(feedItem{"A real model story about inference", "https://ai.example/ok", "", now.Add(-time.Hour)}),
	})
	store := NewStore(t.TempDir())
	store.Now = func() time.Time { return now }
	store.Sources = []Source{
		{ID: "ok", Name: "OK", Group: GroupAI, Tier: "T1", Lang: "en", Feed: srv.URL + "/ok.xml"},
		{ID: "gone", Name: "Gone", Group: GroupMusic, Tier: "T2", Lang: "en", Feed: srv.URL + "/missing.xml"},
	}

	d, err := store.Refresh(context.Background(), true)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if d.SourceFailed != 1 || len(d.Failed) != 1 || d.Failed[0] != "Gone" {
		t.Errorf("failed sources = %v (%d)", d.Failed, d.SourceFailed)
	}
	if len(d.Items) == 0 {
		t.Error("a failing source must not empty the digest")
	}
}

func TestRefreshFailsWithoutAnyItem(t *testing.T) {
	srv, _ := newFeedServer(t, map[string]string{})
	store := NewStore(t.TempDir())
	store.Now = func() time.Time { return testNow() }
	store.Sources = []Source{{ID: "gone", Name: "Gone", Group: GroupAI, Tier: "T1", Lang: "en", Feed: srv.URL + "/none.xml"}}
	if _, err := store.Refresh(context.Background(), true); err == nil {
		t.Fatal("expected an error when every source fails")
	}
}

func TestRefreshReusesTodaysDigest(t *testing.T) {
	now := testNow()
	srv, hits := newFeedServer(t, map[string]string{
		"ai.xml": rssDoc(feedItem{"A model release worth reading about", "https://ai.example/x", "", now.Add(-time.Hour)}),
	})
	store := NewStore(t.TempDir())
	store.Now = func() time.Time { return now }
	store.Sources = []Source{{ID: "ai", Name: "AI", Group: GroupAI, Tier: "T1", Lang: "en", Feed: srv.URL + "/ai.xml"}}

	if _, err := store.Refresh(context.Background(), true); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	first := atomic.LoadInt32(hits)
	if _, err := store.Refresh(context.Background(), false); err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if got := atomic.LoadInt32(hits); got != first {
		t.Errorf("second refresh fetched again: %d -> %d", first, got)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir())
	now := testNow()
	for _, day := range []string{"2026-10-28", "2026-10-29", "2026-10-30"} {
		d := &Digest{Date: day, GeneratedAt: now.Format(time.RFC3339), Items: []Item{{
			ID: itemID("https://example.com/" + day), URL: "https://example.com/" + day,
			Title: "Story " + day, Group: GroupAI, Kind: KindNews, Source: "AI", SourceCount: 1,
		}}}
		if err := store.save(d); err != nil {
			t.Fatalf("save %s: %v", day, err)
		}
	}

	latest, err := store.Latest()
	if err != nil || latest.Date != "2026-10-30" {
		t.Fatalf("Latest = %v, %v", latest, err)
	}
	got, err := store.Get("2026-10-28")
	if err != nil || len(got.Items) != 1 {
		t.Fatalf("Get: %v %v", got, err)
	}
	if _, err := store.Get("../etc/passwd"); err == nil {
		t.Error("invalid day was accepted")
	}
	all, err := store.Days()
	if err != nil || len(all) != 3 || all[0] != "2026-10-30" {
		t.Fatalf("Days = %v %v", all, err)
	}
	store.prune(filepath.Join(store.Dir, "hot"), 2)
	after, _ := store.Days()
	if len(after) != 2 || after[0] != "2026-10-30" || after[1] != "2026-10-29" {
		t.Errorf("after prune = %v", after)
	}
}

func TestSaveWritesLatestAndDay(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.save(&Digest{Date: "2026-10-30", Items: []Item{{ID: "x"}}}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"latest.json", "2026-10-30.json"} {
		if _, err := os.Stat(filepath.Join(dir, "hot", name)); err != nil {
			t.Errorf("%s missing: %v", name, err)
		}
	}
}
