// Package githubdigest builds a daily, locally stored digest of trending
// open-source projects from GitHub: a hardware-focused section (KiCad, PCB,
// embedded, FPGA, RISC-V) plus highly popular projects from other fields.
//
// GitHub has no official "trending" API, so this package queries the public
// search API, merges and de-duplicates the results, ranks them, and stores the
// result as local JSON snapshots (one per day plus latest.json).
package githubdigest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Group marks which section a repository belongs to.
const (
	GroupHardware = "hardware"
	GroupAI       = "ai"
	GroupMusic    = "music"
	GroupGeneral  = "general"
)

// GroupOrder is the display order of the digest sections.
var GroupOrder = []string{GroupHardware, GroupAI, GroupMusic, GroupGeneral}

// GroupTitle is the heading shown for a section.
func GroupTitle(group string) string {
	switch group {
	case GroupHardware:
		return "硬件"
	case GroupAI:
		return "AI"
	case GroupMusic:
		return "音乐"
	default:
		return "其他热门"
	}
}

// Item is one repository in a digest.
type Item struct {
	Name        string   `json:"name"`
	FullName    string   `json:"fullName"`
	URL         string   `json:"url"`
	Description string   `json:"description"`
	Stars       int      `json:"stars"`
	Forks       int      `json:"forks"`
	Language    string   `json:"language"`
	Topics      []string `json:"topics"`
	PushedAt    string   `json:"pushedAt"`
	CreatedAt   string   `json:"createdAt"`
	Avatar      string   `json:"avatar"`
	Group       string   `json:"group"`
}

// Digest is one day of curated projects.
type Digest struct {
	Date        string `json:"date"`
	GeneratedAt string `json:"generatedAt"`
	Items       []Item `json:"items"`
}

// Query describes one GitHub repository search.
type Query struct {
	Name    string
	Group   string
	Kind    string // "active": pushed recently; "new": created recently
	Q       string
	PerPage int
	Sort    string // "stars" (default) or "updated"; updated makes the pool churn daily
}

// DefaultQueries is the full query set used when a GitHub token is configured
// (30 searches/minute). Without a token the search budget is 10/minute, so
// DefaultQueriesCompact keeps the strongest queries of every section instead.
func DefaultQueries() []Query {
	return []Query{
		// 硬件
		{"kicad", GroupHardware, "active", "topic:kicad", 30, "stars"},
		{"pcb", GroupHardware, "active", "topic:pcb", 40, "stars"},
		{"embedded", GroupHardware, "active", "topic:embedded", 50, "stars"},
		{"fpga", GroupHardware, "active", "topic:fpga", 30, "stars"},
		{"riscv", GroupHardware, "active", "topic:riscv", 30, "stars"},
		{"embedded-active", GroupHardware, "active", "topic:embedded stars:>=400", 40, "updated"},
		// AI
		{"llm", GroupAI, "active", "topic:llm stars:>=800", 40, "stars"},
		{"ai", GroupAI, "active", "topic:artificial-intelligence stars:>=800", 40, "stars"},
		{"ai-active", GroupAI, "active", "topic:machine-learning stars:>=2000", 40, "updated"},
		// 音乐
		{"music", GroupMusic, "active", "topic:music stars:>=250", 40, "stars"},
		{"music-active", GroupMusic, "active", "topic:music-player stars:>=250", 40, "updated"},
		// 其他热门
		{"hot-week", GroupGeneral, "week", "stars:>=100", 50, "stars"},
		{"hot-new", GroupGeneral, "new", "stars:>=500", 50, "stars"},
		{"hot-all", GroupGeneral, "active", "stars:>=12000", 30, "stars"},
	}
}

// DefaultQueriesCompact fits the unauthenticated budget of 10 searches/minute.
func DefaultQueriesCompact() []Query {
	return []Query{
		{"embedded", GroupHardware, "active", "topic:embedded", 50, "stars"},
		{"kicad", GroupHardware, "active", "topic:kicad", 30, "stars"},
		{"embedded-active", GroupHardware, "active", "topic:embedded stars:>=400", 40, "updated"},
		{"llm", GroupAI, "active", "topic:llm stars:>=800", 40, "stars"},
		{"ai-active", GroupAI, "active", "topic:machine-learning stars:>=2000", 40, "updated"},
		{"music", GroupMusic, "active", "topic:music stars:>=250", 40, "stars"},
		{"music-active", GroupMusic, "active", "topic:music-player stars:>=250", 40, "updated"},
		{"hot-week", GroupGeneral, "week", "stars:>=100", 50, "stars"},
		{"hot-new", GroupGeneral, "new", "stars:>=500", 50, "stars"},
	}
}

const (
	maxItems = 16
	// Niche topics such as kicad or pcb top out at a few hundred stars, but a
	// digest that mixes them with 16k-star projects still needs a floor so the
	// section stays "popular" rather than merely new.
	minHardwareStars = 250
)

// groupQuota is the nominal size of each section. Shortfalls are topped up from
// the other sections, so these are targets rather than hard limits.
var groupQuota = map[string]int{
	GroupHardware: 6,
	GroupAI:       4,
	GroupMusic:    2,
	GroupGeneral:  4,
}

var dayRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Store reads and writes digest snapshots under Dir and queries the GitHub API.
type Store struct {
	Dir     string
	APIBase string
	Token   string
	Queries []Query
	Client  *http.Client
	Delay   time.Duration
	// RetryDelay is how long to wait before retrying a rate-limited query. It
	// spans a full rate-limit window so the retry lands in a fresh budget.
	// Zero disables the wait (tests).
	RetryDelay time.Duration

	mu     sync.Mutex
	lastAt time.Time
}

// NewStore returns a store writing under dir. token may be empty; setting a
// GitHub token raises the search API rate limit from 10 to 30 requests/minute.
func NewStore(dir, token string) *Store {
	return &Store{
		Dir:     dir,
		APIBase: "https://api.github.com",
		Token:   token,
		Delay:   1500 * time.Millisecond,
		Client:  &http.Client{Timeout: 45 * time.Second},
	}
}

// RunDaily starts a background loop that creates one digest per local day.
// It never blocks the caller. onNew runs after a newly created digest.
func (s *Store) RunDaily(ctx context.Context, onNew func(*Digest)) {
	go func() {
		s.refreshIfNeeded(ctx, onNew)
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refreshIfNeeded(ctx, onNew)
			}
		}
	}()
}

func (s *Store) refreshIfNeeded(ctx context.Context, onNew func(*Digest)) {
	latest, err := s.Latest()
	if err == nil && latest != nil {
		if latest.Date == today() {
			return
		}
		// Do not fetch a new day in the middle of the night; wait until 07:00.
		if time.Now().Hour() < 7 {
			return
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("githubdigest: read latest: %v", err)
	}
	d, err := s.Refresh(ctx, false)
	if err != nil {
		log.Printf("githubdigest: refresh failed: %v", err)
		return
	}
	if d != nil && onNew != nil {
		onNew(d)
	}
}

func today() string { return time.Now().Format("2006-01-02") }

func (s *Store) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 45 * time.Second}
}

func (s *Store) queries() []Query {
	if len(s.Queries) > 0 {
		return s.Queries
	}
	if s.Token != "" {
		return DefaultQueries()
	}
	return DefaultQueriesCompact()
}

// Refresh fetches a new digest. force bypasses the 10-minute cooldown that
// protects the unauthenticated GitHub search rate limit.
func (s *Store) Refresh(ctx context.Context, force bool) (*Digest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	day := today()
	if latest, err := s.readLatest(); err == nil && latest != nil {
		if latest.Date == day && !force {
			return latest, nil
		}
		if !force && !s.lastAt.IsZero() && time.Since(s.lastAt) < 10*time.Minute {
			return latest, nil
		}
	}

	queries := s.queries()
	sinceActive := time.Now().AddDate(0, 0, -14).Format("2006-01-02")
	sinceNew := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	sinceWeek := time.Now().AddDate(0, 0, -7).Format("2006-01-02")

	type queryResult struct {
		items []Item
		err   error
	}
	results := make([]queryResult, len(queries))
	var wg sync.WaitGroup

	for i, q := range queries {
		// Stagger launches so a burst never trips GitHub's secondary rate limit.
		if i > 0 && s.Delay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(i) * s.Delay):
			}
		}
		qualifier := "pushed:>=" + sinceActive
		switch q.Kind {
		case "new":
			qualifier = "created:>=" + sinceNew
		case "week":
			qualifier = "created:>=" + sinceWeek
		}
		wg.Add(1)
		go func(idx int, q Query, qualifier string) {
			defer wg.Done()
			items, err := s.search(ctx, q.Q+" "+qualifier, q.PerPage, q.Sort)
			results[idx] = queryResult{items: items, err: err}
		}(i, q, qualifier)
	}
	wg.Wait()

	seen := make(map[string]Item)
	pools := make(map[string][]Item, len(GroupOrder))
	for i, q := range queries {
		r := results[i]
		if r.err != nil {
			log.Printf("githubdigest: query %q failed: %v", q.Name, r.err)
			continue
		}
		for _, it := range r.items {
			if it.FullName == "" || it.URL == "" {
				continue
			}
			if _, dup := seen[it.FullName]; dup {
				continue
			}
			// Drop star-farmed repositories: implausibly few forks for the
			// number of stars recorded on a very young project.
			if !plausiblePopularity(it) {
				continue
			}
			relevant := isHardwareRelevant(it)
			switch q.Group {
			case GroupHardware:
				// Keep the hardware section genuinely popular and on topic.
				if !relevant || it.Stars < minHardwareStars {
					continue
				}
			case GroupGeneral:
				// A hardware repository belongs in the hardware section only.
				if relevant {
					continue
				}
			}
			it.Group = q.Group
			pools[q.Group] = append(pools[q.Group], it)
			seen[it.FullName] = it
		}
	}

	total := 0
	for _, g := range GroupOrder {
		total += len(pools[g])
	}
	if total == 0 {
		return nil, errors.New("no repositories returned")
	}

	// Prefer repositories that did not appear in the last ten digests, so the
	// daily feed stays fresh instead of repeating the same famous projects.
	recent := s.recentNames(10)
	picked := make(map[string][]Item, len(GroupOrder))
	count := 0
	for _, g := range GroupOrder {
		sortByScore(pools[g])
		picked[g] = pickFresh(pools[g], groupQuota[g], recent)
		count += len(picked[g])
	}
	// Top up any shortfall from the sections that still have candidates.
	for _, g := range GroupOrder {
		if count >= maxItems {
			break
		}
		extra := fillFrom(pools[g], picked[g], maxItems-count, recent)
		picked[g] = append(picked[g], extra...)
		count += len(extra)
	}

	items := make([]Item, 0, count)
	for _, g := range GroupOrder {
		items = append(items, picked[g]...)
	}

	d := &Digest{
		Date:        day,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Items:       items,
	}
	if err := s.save(d); err != nil {
		return nil, err
	}
	s.lastAt = time.Now()
	return d, nil
}

// fillFrom returns up to need items from pool that are not already in taken.
func fillFrom(pool, taken []Item, need int, recent map[string]int) []Item {
	if need <= 0 || len(pool) == 0 {
		return nil
	}
	have := make(map[string]bool, len(taken))
	for _, it := range taken {
		have[it.FullName] = true
	}
	rest := make([]Item, 0, len(pool))
	for _, it := range pool {
		if !have[it.FullName] {
			rest = append(rest, it)
		}
	}
	// pickFresh keeps unseen candidates ahead of stale ones, so topping up here
	// never reaches for a recently shown repository while fresh ones remain.
	return pickFresh(rest, need, recent)
}

func appendUnique(dst, src []Item) []Item {
	seen := make(map[string]bool, len(dst))
	for _, it := range dst {
		seen[it.FullName] = true
	}
	for _, it := range src {
		if seen[it.FullName] {
			continue
		}
		seen[it.FullName] = true
		dst = append(dst, it)
	}
	return dst
}

// pickFresh returns up to n items, preferring ones absent from recent digests.
// recent maps a repository to how many digests ago it last appeared.
func pickFresh(list []Item, n int, recent map[string]int) []Item {
	if n <= 0 || len(list) == 0 {
		return nil
	}
	out := make([]Item, 0, n)
	taken := make(map[string]bool, n)
	for _, it := range list {
		if len(out) == n {
			break
		}
		if _, ok := recent[it.FullName]; ok {
			continue
		}
		out = append(out, it)
		taken[it.FullName] = true
	}
	if len(out) == n {
		return out
	}
	// The unseen pool ran dry: reuse repositories, but always the ones that
	// were shown longest ago, so a repeat keeps drifting instead of sticking.
	rest := make([]Item, 0, len(list))
	for _, it := range list {
		if taken[it.FullName] {
			continue
		}
		rest = append(rest, it)
	}
	sort.SliceStable(rest, func(i, j int) bool {
		return recent[rest[i].FullName] > recent[rest[j].FullName]
	})
	for _, it := range rest {
		if len(out) == n {
			break
		}
		out = append(out, it)
	}
	return out
}

func sortByScore(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		si, sj := score(items[i]), score(items[j])
		if si != sj {
			return si > sj
		}
		if items[i].Stars != items[j].Stars {
			return items[i].Stars > items[j].Stars
		}
		return items[i].FullName < items[j].FullName
	})
}

type searchResponse struct {
	Items []struct {
		FullName    string   `json:"full_name"`
		Name        string   `json:"name"`
		HTMLURL     string   `json:"html_url"`
		Description string   `json:"description"`
		Stars       int      `json:"stargazers_count"`
		Forks       int      `json:"forks_count"`
		Language    string   `json:"language"`
		Topics      []string `json:"topics"`
		PushedAt    string   `json:"pushed_at"`
		CreatedAt   string   `json:"created_at"`
		Owner       struct {
			AvatarURL string `json:"avatar_url"`
		} `json:"owner"`
	} `json:"items"`
}

func (s *Store) search(ctx context.Context, query string, perPage int, sortBy string) ([]Item, error) {
	if perPage <= 0 || perPage > 100 {
		perPage = 20
	}
	if sortBy == "" {
		sortBy = "stars"
	}
	endpoint := strings.TrimRight(s.APIBase, "/") +
		"/search/repositories?q=" + url.QueryEscape(query) +
		"&sort=" + url.QueryEscape(sortBy) + "&order=desc&per_page=" + strconv.Itoa(perPage)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "qdu-wasteland-open-source-digest/1.0")
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	// One backoff retry keeps a single rate-limited query from silently
	// degrading the day's digest; the unauthenticated budget is only 10
	// searches per minute and this run issues nine.
	if (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) && s.Token == "" {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		resp.Body.Close()
		if strings.Contains(strings.ToLower(string(body)), "rate limit") {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(s.RetryDelay):
			}
			retry, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if err != nil {
				return nil, err
			}
			retry.Header = req.Header
			resp, err = s.client().Do(retry)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("github returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("github returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed searchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&parsed); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(parsed.Items))
	for _, it := range parsed.Items {
		desc := strings.TrimSpace(it.Description)
		if len(desc) > 240 {
			desc = desc[:240] + "…"
		}
		out = append(out, Item{
			Name:        it.Name,
			FullName:    it.FullName,
			URL:         it.HTMLURL,
			Description: desc,
			Stars:       it.Stars,
			Forks:       it.Forks,
			Language:    it.Language,
			Topics:      it.Topics,
			PushedAt:    it.PushedAt,
			CreatedAt:   it.CreatedAt,
			Avatar:      it.Owner.AvatarURL,
		})
	}
	return out, nil
}

// strongTerms must appear in a repository's name or description before it can
// be treated as hardware. Topic tags alone are not enough: unrelated projects
// sometimes tag themselves with "pcb" or "cad" for visibility.
var strongTerms = []string{
	"kicad", "gerber", "schematic", "pcb", "embedded", "firmware", "mcu",
	"stm32", "esp32", "esp8266", "arduino", "fpga", "verilog", "vhdl",
	"risc-v", "riscv", "microcontroller", "rtos", "electronics", "circuit",
	"sensor", "jtag", "swd", "modbus", "canbus", "oscilloscope",
}

// veryStrongTerms override the software-agent exclusion below.
var veryStrongTerms = []string{
	"kicad", "gerber", "schematic", "fpga", "verilog", "vhdl", "rtos",
	"firmware", "microcontroller", "stm32", "esp32", "esp8266", "arduino", "pcb",
}

// softwareSignals mark descriptions that are usually software/AI projects
// riding on hardware-adjacent tags.
var softwareSignals = []string{
	"coding agent", "ai agent", "agent framework", "multi-agent", "llm",
	"language model", "prompt engineering", "chatbot", "web framework",
}

// plausiblePopularity rejects repositories whose star count is out of
// proportion with their fork count, a common pattern for star-farming.
func plausiblePopularity(it Item) bool {
	if it.Stars < 5000 {
		return true
	}
	return it.Forks*200 >= it.Stars
}

func isHardwareRelevant(it Item) bool {
	text := strings.ToLower(it.Name + " " + it.Description)
	hasStrong := false
	for _, w := range strongTerms {
		if strings.Contains(text, w) {
			hasStrong = true
			break
		}
	}
	if !hasStrong {
		return false
	}
	for _, w := range softwareSignals {
		if !strings.Contains(text, w) {
			continue
		}
		for _, v := range veryStrongTerms {
			if strings.Contains(text, v) {
				return true
			}
		}
		return false
	}
	return true
}

var hardwareWords = []string{
	"kicad", "pcb", "gerber", "schematic", "embedded", "firmware", "mcu",
	"stm32", "esp32", "esp8266", "arduino", "fpga", "verilog", "vhdl",
	"risc-v", "riscv", "sensor", "rtos", "electronics", "hardware", "jtag",
	"swd", "canbus", "modbus", "raspberry", "microcontroller", "circuit",
	"eda", "drone", "robotics", "robot", "oscilloscope", "logic analyzer",
	"asic", "serial", "usb", "i2c", "spi", "uart",
}

// score ranks a repository. The hardware section favours obvious hardware
// projects; the general section favours star velocity (stars per day since the
// repository was created), which surfaces currently hot projects instead of the
// same all-time famous repositories every day.
func score(it Item) float64 {
	if it.Group == GroupAI || it.Group == GroupGeneral {
		// Star velocity: rising projects beat the same all-time giants. The AI
		// field moves fast enough that this is what "hot" means there.
		days := 3650.0
		if t, err := time.Parse(time.RFC3339, it.CreatedAt); err == nil {
			if d := time.Since(t).Hours() / 24; d >= 1 {
				days = d
			}
		}
		return math.Log10(float64(it.Stars)/days + 1)
	}
	if it.Group == GroupMusic {
		// Music tooling matures slowly and the pool is small: rank by reach.
		value := math.Log10(float64(it.Stars) + 1)
		if t, err := time.Parse(time.RFC3339, it.PushedAt); err == nil && time.Since(t) < 72*time.Hour {
			value += 0.6
		}
		return value
	}
	text := strings.ToLower(it.FullName + " " + it.Description + " " + strings.Join(it.Topics, " "))
	value := math.Log10(float64(it.Stars) + 1)
	for _, w := range hardwareWords {
		if strings.Contains(text, w) {
			value += 1.5
		}
	}
	if t, err := time.Parse(time.RFC3339, it.PushedAt); err == nil && time.Since(t) < 72*time.Hour {
		value += 0.8
	}
	return value
}

func (s *Store) digestDir() string { return filepath.Join(s.Dir, "digest") }

func (s *Store) save(d *Digest) error {
	dir := s.digestDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(dir, d.Date+".json"), raw); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(dir, "latest.json"), raw); err != nil {
		return err
	}
	s.prune(dir, 60)
	return nil
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func (s *Store) prune(dir string, keep int) {
	days, err := listDays(dir)
	if err != nil || len(days) <= keep {
		return
	}
	sort.Strings(days)
	for _, day := range days[:len(days)-keep] {
		_ = os.Remove(filepath.Join(dir, day+".json"))
	}
}

// recentNames collects repository names from the previous n daily digests so
// the new digest can prefer projects that have not been shown recently.
// recentNames maps repository full names shown in the last n digests (today
// excluded) to how many digests ago they appeared. Smaller means more recent.
func (s *Store) recentNames(n int) map[string]int {
	out := make(map[string]int)
	days, err := s.Days()
	if err != nil {
		return out
	}
	count := 0
	for _, day := range days {
		if day == today() {
			continue
		}
		d, err := s.Get(day)
		if err != nil {
			continue
		}
		count++
		for _, it := range d.Items {
			if prev, ok := out[it.FullName]; !ok || count < prev {
				out[it.FullName] = count
			}
		}
		if count >= n {
			break
		}
	}
	return out
}

func listDays(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var days []string
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".json")
		if e.IsDir() || name == e.Name() || !dayRe.MatchString(name) {
			continue
		}
		days = append(days, name)
	}
	return days, nil
}

// Latest returns the most recent digest.
func (s *Store) Latest() (*Digest, error) {
	return s.readLatest()
}

func (s *Store) readLatest() (*Digest, error) {
	return readDigest(filepath.Join(s.digestDir(), "latest.json"))
}

// Get returns the digest for a specific YYYY-MM-DD day.
func (s *Store) Get(day string) (*Digest, error) {
	if !dayRe.MatchString(day) {
		return nil, errors.New("invalid date")
	}
	return readDigest(filepath.Join(s.digestDir(), day+".json"))
}

// Days returns saved days, newest first.
func (s *Store) Days() ([]string, error) {
	days, err := listDays(s.digestDir())
	if err != nil {
		return nil, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	return days, nil
}

func readDigest(path string) (*Digest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d Digest
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return &d, nil
}
