// Package hotdigest builds a daily, locally stored digest of hot topics in the
// six fields this site follows: philosophy theory, essays, power electronics,
// embedded hardware, AI and music.
//
// The pipeline follows the shape of the open-source AIHOT project
// (github.com/KKKKhazix/AIHOT): a graded source list is collected, obvious
// marketing is prescreened away, reports of the same story are clustered into
// one event, heat is computed per event rather than per article, and the result
// is written as a daily issue. Unlike AIHOT it runs as one Go package with no
// Node runtime, database or paid service: snapshots are local JSON files next
// to the site's other tool data.
package hotdigest

import (
	"database/sql"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"wasteland/internal/digestdb"
)

// The seven fields of the digest.
const (
	GroupPhilosophy = "phil"
	GroupEssay      = "essay"
	GroupPoetry     = "poetry"
	GroupPower      = "power"
	GroupEmbedded   = "embedded"
	GroupAI         = "ai"
	GroupMusic      = "music"
)

// GroupOrder is the display order of the tabs.
var GroupOrder = []string{GroupPhilosophy, GroupEssay, GroupPoetry, GroupPower, GroupEmbedded, GroupAI, GroupMusic}

// GroupTitle is the Chinese heading shown for a section.
func GroupTitle(group string) string {
	switch group {
	case GroupPhilosophy:
		return "哲学理论"
	case GroupEssay:
		return "思想散文"
	case GroupPoetry:
		return "当代诗歌"
	case GroupPower:
		return "电力电子"
	case GroupEmbedded:
		return "嵌入式"
	case GroupAI:
		return "AI"
	case GroupMusic:
		return "潮流音乐"
	default:
		return "其他"
	}
}

const (
	dayLayout = "2006-01-02"

	// maxPerDigest bounds one issue across all sections as a safety net.
	maxPerDigest = 400
	// recentDigests is how many previous issues a story must skip before it
	// may be shown again.
	recentDigests = 3
	// cooldown protects the sources from repeated fetch bursts.
	cooldown = 10 * time.Minute
	// curationConcurrency bounds parallel model calls per run.
	curationConcurrency = 4

	userAgent = "qdu-wasteland-hot-digest/1.0 (+https://github.com/youyan2000/QDU_Wasteland)"
)

// groupSpec tunes one section: how many items it carries, how far back it may
// look, and whether press coverage and journal papers have to coexist.
type groupSpec struct {
	// min and target are the floor and the usual size of the section; max is a
	// hard stop. The section is never padded with off-topic items to reach min.
	min    int
	target int
	max    int
	// window is how far back a press item may be published.
	window time.Duration
	// journalWindow is how far back a journal may be searched. Journals publish
	// monthly, so it is wider than the news window.
	journalWindow time.Duration
	// perSource caps how many items one feed or journal may contribute.
	perSource int
	// mixedKinds requires papers and press coverage to both be represented.
	mixedKinds bool
	// minKindShare is the share reserved for the minority kind.
	minKindShare float64
	// splitTracks splits the field into the three language tracks.
	splitTracks bool
}

var groupSpecs = map[string]groupSpec{
	// Journals publish slowly, so the theory section looks back a quarter and
	// rotates through it instead of pretending each item is from today.
	GroupPhilosophy: {min: 20, target: 30, max: 50, window: 90 * 24 * time.Hour, journalWindow: 120 * 24 * time.Hour, perSource: 3, splitTracks: true},
	GroupEssay:      {min: 20, target: 30, max: 50, window: 30 * 24 * time.Hour, perSource: 6, splitTracks: true},
	GroupPoetry:     {min: 20, target: 30, max: 50, window: 60 * 24 * time.Hour, perSource: 6, splitTracks: true},
	// The two engineering sections must mix vendor/community news with papers;
	// a monthly journal needs a much wider search window than a news feed.
	GroupPower:    {min: 20, target: 30, max: 50, window: 14 * 24 * time.Hour, journalWindow: 75 * 24 * time.Hour, perSource: 4, mixedKinds: true, minKindShare: 0.33},
	GroupEmbedded: {min: 20, target: 30, max: 50, window: 14 * 24 * time.Hour, journalWindow: 75 * 24 * time.Hour, perSource: 3, mixedKinds: true, minKindShare: 0.33},
	GroupAI:       {min: 20, target: 30, max: 50, window: 10 * 24 * time.Hour, perSource: 3},
	GroupMusic:    {min: 20, target: 30, max: 50, window: 21 * 24 * time.Hour, perSource: 4},
}

// IsSplit reports whether a field is divided into language tracks.
func IsSplit(group string) bool { return groupSpecs[group].splitTracks }

// sectionKey identifies one selectable list: a field, or one language track of
// a split field.
func sectionKey(group, track string) string {
	if track == "" {
		return group
	}
	return group + "/" + track
}

// section is one selectable list of the digest.
type section struct {
	Key   string
	Group string
	Track string
}

// sections lists every selectable list in display order.
func sections() []section {
	out := make([]section, 0, len(GroupOrder)+6)
	for _, group := range GroupOrder {
		if !groupSpecs[group].splitTracks {
			out = append(out, section{Key: group, Group: group})
			continue
		}
		for _, track := range TrackOrder {
			out = append(out, section{Key: sectionKey(group, track), Group: group, Track: track})
		}
	}
	return out
}

// Item is one story in a digest. Clustered reports of the same story are
// folded into one item; Sources lists the other outlets that covered it.
type Item struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	TitleOrig   string   `json:"titleOriginal,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	URL         string   `json:"url"`
	Group       string   `json:"group"`
	Track       string   `json:"track,omitempty"`
	Kind        string   `json:"kind"`
	Source      string   `json:"source"`
	SourceID    string   `json:"sourceId"`
	Sources     []string `json:"sources,omitempty"`
	SourceCount int      `json:"sourceCount"`
	Tier        string   `json:"tier"`
	Lang        string   `json:"lang"`
	Venue       string   `json:"venue,omitempty"`
	Citations   int      `json:"citations,omitempty"`
	// Issue is the journal issue label shown instead of a relative time.
	Issue       string  `json:"issue,omitempty"`
	PublishedAt string  `json:"publishedAt"`
	Heat        float64 `json:"heat"`
	Rewritten   bool    `json:"rewritten,omitempty"`
	// Translated marks a title rewritten across languages, so the page can tell
	// a translation from a same-language rephrasing.
	Translated bool `json:"translated,omitempty"`

	// sourceMax is the per-source cap that applies to this item during
	// selection; it is not serialised.
	sourceMax int
}

// Digest is one day of curated stories.
type Digest struct {
	Date         string   `json:"date"`
	GeneratedAt  string   `json:"generatedAt"`
	SourceTotal  int      `json:"sourceTotal"`
	SourceFailed int      `json:"sourceFailed"`
	Failed       []string `json:"failedSources,omitempty"`
	Items        []Item   `json:"items"`
}

// Store reads and writes digest snapshots under Dir and fetches the sources.
type Store struct {
	// DB 非空时快照写入 SQLite（表 hot_issues），否则退回 tool-data 下的 JSON 文件。
	DB *sql.DB
	Dir     string
	Sources []Source
	Client  *http.Client
	// Enrich rewrites titles and summaries; nil keeps the source wording.
	Enrich *Enricher
	// Now is the clock, overridable in tests.
	Now func() time.Time
	// Concurrency bounds parallel source requests.
	Concurrency int

	mu     sync.Mutex
	lastAt time.Time

	// Crossref requests are serialised through these fields.
	crossrefMu sync.Mutex
	crossrefAt time.Time
}

// NewStore returns a store that writes under dir.
func NewStore(dir string) *Store {
	return &Store{
		Dir:         dir,
		Sources:     DefaultSources(),
		Client:      &http.Client{Timeout: 30 * time.Second},
		Now:         time.Now,
		Concurrency: 8,
	}
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Store) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (s *Store) sources() []Source {
	if len(s.Sources) > 0 {
		return s.Sources
	}
	return DefaultSources()
}

// RunDaily starts a background loop that creates one digest per local day.
// It never blocks the caller; onNew runs after a newly created digest.
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
		if latest.Date == s.now().Format(dayLayout) {
			return
		}
		// Do not build a new day in the middle of the night; wait until 07:00.
		if s.now().Hour() < 7 {
			return
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("hotdigest: read latest: %v", err)
	}
	d, err := s.Refresh(ctx, false)
	if err != nil {
		log.Printf("hotdigest: refresh failed: %v", err)
		return
	}
	if d != nil && onNew != nil {
		onNew(d)
	}
}

// Refresh fetches every source and writes a new digest. force bypasses the
// same-day and cooldown short-circuits that protect the sources.
func (s *Store) Refresh(ctx context.Context, force bool) (*Digest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	day := now.Format(dayLayout)
	if latest, err := s.readLatest(); err == nil && latest != nil {
		if latest.Date == day && !force {
			return latest, nil
		}
		if !force && !s.lastAt.IsZero() && now.Sub(s.lastAt) < cooldown {
			return latest, nil
		}
	}

	results := s.fetchAll(ctx, s.sources(), now)
	pools, failed := s.buildPools(results, now)

	total := 0
	for _, g := range GroupOrder {
		total += len(pools[g])
	}
	if total == 0 {
		return nil, errors.New("no usable items returned by any source")
	}

	items := s.selectAndCurate(ctx, pools, s.recentIDs(recentDigests))

	d := &Digest{
		Date:         day,
		GeneratedAt:  now.Format(time.RFC3339),
		SourceTotal:  len(results),
		SourceFailed: len(failed),
		Failed:       failed,
		Items:        items,
	}
	if err := s.save(d); err != nil {
		return nil, err
	}
	s.lastAt = now
	return d, nil
}

type sourceResult struct {
	src   Source
	items []rawItem
	err   error
}

// fetchAll fetches every source with bounded parallelism. A failing source is
// reported and skipped; it never aborts the day's digest.
func (s *Store) fetchAll(ctx context.Context, sources []Source, now time.Time) []sourceResult {
	results := make([]sourceResult, len(sources))
	limit := s.Concurrency
	if limit <= 0 {
		limit = 8
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, src := range sources {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, src Source) {
			defer wg.Done()
			defer func() { <-sem }()
			var items []rawItem
			var err error
			if src.ISSN != "" {
				window := groupSpecs[src.Group].journalWindow
				if window <= 0 {
					window = 60 * 24 * time.Hour
				}
				items, err = s.fetchJournal(ctx, src, now.Add(-window))
			} else {
				items, err = s.fetchFeed(ctx, src)
			}
			results[i] = sourceResult{src: src, items: items, err: err}
		}(i, src)
	}
	wg.Wait()
	return results
}

func (s *Store) fetchFeed(ctx context.Context, src Source) ([]rawItem, error) {
	if strings.TrimSpace(src.Feed) == "" {
		return nil, errors.New("source has no feed URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.Feed, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml;q=0.9, */*;q=0.5")
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return parseFeed(resp.Body)
}

// buildPools turns raw entries into ranked, clustered per-group pools.
func (s *Store) buildPools(results []sourceResult, now time.Time) (map[string][]Item, []string) {
	pools := make(map[string][]Item, len(GroupOrder))
	failed := make([]string, 0, 8)
	seen := make(map[string]bool)
	candidates := make(map[string][]*candidate, len(GroupOrder))

	for _, res := range results {
		if res.err != nil {
			log.Printf("hotdigest: source %q failed: %v", res.src.ID, res.err)
			failed = append(failed, res.src.Name)
			continue
		}
		spec := groupSpecs[res.src.Group]
		window := spec.window
		if window <= 0 {
			window = 14 * 24 * time.Hour
		}
		paperWindow := spec.journalWindow
		if paperWindow <= 0 {
			paperWindow = window
		}
		newsCutoff := now.Add(-window)
		paperCutoff := now.Add(-paperWindow)
		for _, raw := range res.items {
			title := stripSiteSuffix(raw.Title, res.src.Name)
			if title == "" || looksLikeNoise(title) || looksLowValue(title) || isBookkeeping(title) {
				continue
			}
			link := canonicalURL(raw.Link)
			if link == "" || seen[link] {
				continue
			}
			if raw.Published.IsZero() {
				// Without a publication date an entry cannot be ranked or
				// checked for freshness, so it stays out.
				continue
			}
			ts := raw.Published.Local()
			if ts.After(now.Add(2 * time.Hour)) {
				ts = now
			}
			// A journal article keeps its relevance far longer than a news
			// item, so papers are judged against the journal window.
			if raw.Kind == KindPaper {
				if ts.Before(paperCutoff) {
					continue
				}
			} else if ts.Before(newsCutoff) {
				continue
			}
			if res.src.Broad && !onTopic(res.src.Group, title+" "+raw.Summary) {
				continue
			}
			seen[link] = true
			kind := raw.Kind
			if kind == "" {
				kind = KindNews
			}
			key := sectionKey(res.src.Group, res.src.Track)
			candidates[key] = append(candidates[key], &candidate{
				item: Item{
					ID:          itemID(link),
					Title:       title,
					Summary:     raw.Summary,
					URL:         link,
					Group:       res.src.Group,
					Track:       res.src.Track,
					Kind:        kind,
					Source:      res.src.Name,
					SourceID:    res.src.ID,
					Tier:        res.src.Tier,
					Lang:        res.src.Lang,
					Venue:       raw.Venue,
					Citations:   raw.Citations,
					Issue:       raw.Issue,
					PublishedAt: ts.Format(time.RFC3339),
					sourceMax:   res.src.Max,
				},
				src:    res.src,
				ts:     ts,
				tokens: titleTokens(title),
			})
		}
	}

	for _, sec := range sections() {
		pool := candidates[sec.Key]
		if len(pool) == 0 {
			pools[sec.Key] = nil
			continue
		}
		sort.SliceStable(pool, func(i, j int) bool {
			return pool[i].ts.After(pool[j].ts)
		})
		if len(pool) > maxPoolPerGroup {
			pool = pool[:maxPoolPerGroup]
		}
		clusters := clusterPool(pool)
		items := make([]Item, 0, len(clusters))
		for _, cl := range clusters {
			item := finalizeCluster(cl)
			item.Heat = heat(item, cl.rep.src, cl.rep.ts, now)
			items = append(items, item)
		}
		sortItems(items)
		pools[sec.Key] = items
	}
	sort.Strings(failed)
	return pools, failed
}

// sortItems orders a section by heat, then recency, then title.
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Heat != items[j].Heat {
			return items[i].Heat > items[j].Heat
		}
		if items[i].PublishedAt != items[j].PublishedAt {
			return items[i].PublishedAt > items[j].PublishedAt
		}
		return items[i].Title < items[j].Title
	})
}

// selectAndCurate fills every selectable list independently. With a model
// configured it picks from a wider candidate pool and writes the titles;
// otherwise the rule based selection stands. The lists are curated in parallel
// because each call to a reasoning model takes tens of seconds.
func (s *Store) selectAndCurate(ctx context.Context, pools map[string][]Item, recent map[string]int) []Item {
	lists := sections()
	picked := make([][]Item, len(lists))
	for i, sec := range lists {
		picked[i] = selectGroup(pools[sec.Key], groupSpecs[sec.Group], recent)
	}
	// curationConcurrency keeps the gateway from dropping connections when all
	// thirteen lists are curated at once.
	var sem = make(chan struct{}, curationConcurrency)
	if s.Enrich != nil {
		var wg sync.WaitGroup
		for i, sec := range lists {
			if len(picked[i]) == 0 {
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(i int, sec section) {
				defer wg.Done()
				defer func() { <-sem }()
				start := time.Now()
				candidates := candidatesForCuration(pools[sec.Key], curatorMaxCandidates)
				curated := s.Enrich.Curate(ctx, sec.Group, candidates, groupSpecs[sec.Group].target)
				if len(curated) == 0 {
					return
				}
				// The model decides the order and the wording, but it must not
				// shrink a section below the rule baseline, and it must respect
				// the per-source cap the rules enforce.
				picked[i] = capBySource(mergeCurated(curated, picked[i], groupSpecs[sec.Group].target), groupSpecs[sec.Group])
				log.Printf("hotdigest: curated %q to %d items in %s", sec.Key, len(picked[i]), time.Since(start).Round(time.Second))
			}(i, sec)
		}
		wg.Wait()
	}

	out := make([]Item, 0, len(lists)*30)
	for _, items := range picked {
		out = append(out, items...)
	}
	// Everything that is not Chinese must reach the page in English; the
	// curation prompt asks for it, this pass guarantees it.
	if s.Enrich != nil && len(out) > 0 {
		out = s.Enrich.Translate(ctx, out)
	}
	if len(out) > maxPerDigest {
		out = out[:maxPerDigest]
	}
	return out
}

// mergeCurated keeps the model's order and wording, then appends the rule
// selection's remaining items so a cautious model cannot shrink a section.
func mergeCurated(curated, rules []Item, target int) []Item {
	seen := make(map[string]bool, len(curated))
	out := make([]Item, 0, target)
	for _, it := range curated {
		if seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		out = append(out, it)
	}
	for _, it := range rules {
		if len(out) >= target {
			break
		}
		if seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		out = append(out, it)
	}
	return out
}

// capBySource re-applies the per-source limit to a curated list, keeping the
// model's order. Without it a model that likes one journal could fill a whole
// section with a single source.
func capBySource(items []Item, spec groupSpec) []Item {
	counts := make(map[string]int, len(items))
	out := make([]Item, 0, len(items))
	for _, it := range items {
		limit := spec.perSource
		if it.sourceMax > 0 {
			limit = it.sourceMax
		}
		if limit <= 0 {
			limit = 3
		}
		if counts[it.SourceID] >= limit {
			continue
		}
		counts[it.SourceID]++
		out = append(out, it)
	}
	return out
}

// selectGroups fills every section independently: one issue carries each
// section's own list, and a thin section never borrows from another field.
func selectGroups(pools map[string][]Item, recent map[string]int) []Item {
	out := make([]Item, 0, len(GroupOrder)*30)
	for _, group := range GroupOrder {
		out = append(out, selectGroup(pools[group], groupSpecs[group], recent)...)
	}
	if len(out) > maxPerDigest {
		out = out[:maxPerDigest]
	}
	return out
}

// selectGroup picks one section: unseen stories first, per-source and
// per-kind caps respected, and previously shown stories only when the fresh
// pool cannot reach the section's floor.
func selectGroup(pool []Item, spec groupSpec, recent map[string]int) []Item {
	if len(pool) == 0 {
		return nil
	}
	limit := spec.target
	if limit <= 0 {
		limit = 30
	}
	perSource := make(map[string]int)
	kindCount := make(map[string]int)
	taken := make(map[string]bool)
	kindCap := 0
	if spec.mixedKinds && spec.minKindShare > 0 {
		kindCap = int(math.Ceil(float64(limit) * (1 - spec.minKindShare)))
	}

	allow := func(it Item) bool {
		if taken[it.URL] {
			return false
		}
		if kindCap > 0 && kindCount[it.Kind] >= kindCap {
			return false
		}
		cap := spec.perSource
		if it.sourceMax > 0 {
			cap = it.sourceMax
		}
		if cap <= 0 {
			cap = 3
		}
		return perSource[it.SourceID] < cap
	}

	out := make([]Item, 0, limit)
	takeItem := func(it Item) {
		out = append(out, it)
		perSource[it.SourceID]++
		kindCount[it.Kind]++
		taken[it.URL] = true
	}

	for _, it := range pool {
		if len(out) >= limit {
			break
		}
		if _, shown := recent[it.ID]; shown {
			continue
		}
		if !allow(it) {
			continue
		}
		takeItem(it)
	}
	if len(out) >= spec.min || len(out) >= limit {
		return out
	}

	// The unseen pool ran dry: reuse stories, longest-unseen first.
	rest := make([]Item, 0, len(pool))
	for _, it := range pool {
		if _, shown := recent[it.ID]; !shown {
			continue
		}
		if !allow(it) {
			continue
		}
		rest = append(rest, it)
	}
	sort.SliceStable(rest, func(i, j int) bool {
		return recent[rest[i].ID] > recent[rest[j].ID]
	})
	for _, it := range rest {
		if len(out) >= spec.min {
			break
		}
		takeItem(it)
	}
	return out
}

func itemID(canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])[:12]
}

// ---- storage ----

func (s *Store) digestDir() string { return filepath.Join(s.Dir, "hot") }

func (s *Store) save(d *Digest) error {
	if s.DB != nil {
		raw, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			return err
		}
		kv := digestdb.Store{DB: s.DB, Prefix: "hot"}
		if err := kv.Save(d.Date, raw); err != nil {
			return err
		}
		if err := kv.Save("latest", raw); err != nil {
			return err
		}
		return kv.Prune(60)
	}
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
		if e.IsDir() || name == e.Name() || !isDay(name) {
			continue
		}
		days = append(days, name)
	}
	return days, nil
}

func isDay(name string) bool {
	if len(name) != len(dayLayout) {
		return false
	}
	if _, err := time.Parse(dayLayout, name); err != nil {
		return false
	}
	return true
}

// Latest returns the most recent digest.
func (s *Store) Latest() (*Digest, error) { return s.readLatest() }

func (s *Store) readLatest() (*Digest, error) {
	if s.DB != nil {
		raw, err := (digestdb.Store{DB: s.DB, Prefix: "hot"}).Load("latest")
		if err != nil {
			return nil, err
		}
		return decodeDigest(raw)
	}
	return readDigest(filepath.Join(s.digestDir(), "latest.json"))
}

// Get returns the digest for a specific YYYY-MM-DD day.
func (s *Store) Get(day string) (*Digest, error) {
	if !isDay(day) {
		return nil, errors.New("invalid date")
	}
	if s.DB != nil {
		raw, err := (digestdb.Store{DB: s.DB, Prefix: "hot"}).Load(day)
		if err != nil {
			return nil, err
		}
		return decodeDigest(raw)
	}
	return readDigest(filepath.Join(s.digestDir(), day+".json"))
}

// Days returns saved days, newest first.
func (s *Store) Days() ([]string, error) {
	if s.DB != nil {
		return (digestdb.Store{DB: s.DB, Prefix: "hot"}).Names(10)
	}
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

// recentIDs maps a story ID to how many issues ago it was shown. Smaller means
// more recent, so a caller can prefer the longest-unseen story.
func (s *Store) recentIDs(n int) map[string]int {
	out := make(map[string]int)
	days, err := s.Days()
	if err != nil {
		return out
	}
	today := s.now().Format(dayLayout)
	count := 0
	for _, day := range days {
		if day == today {
			continue
		}
		d, err := s.Get(day)
		if err != nil {
			continue
		}
		count++
		for _, it := range d.Items {
			if prev, ok := out[it.ID]; !ok || count < prev {
				out[it.ID] = count
			}
		}
		if count >= n {
			break
		}
	}
	return out
}

// decodeDigest 把库里的 JSON 解成 Digest。
func decodeDigest(raw []byte) (*Digest, error) {
	var d Digest
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return &d, nil
}