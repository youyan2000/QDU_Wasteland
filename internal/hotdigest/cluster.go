package hotdigest

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
)

// maxPoolPerGroup bounds the per-group candidate pool that clustering and
// ranking work on. The newest entries win, because heat decays with age anyway.
const maxPoolPerGroup = 400

// latinStopWords are dropped from title tokens so "the state of AI" and "AI:
// the state of it" stay comparable.
var latinStopWords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "from": true,
	"that": true, "this": true, "its": true, "are": true, "was": true,
	"has": true, "have": true, "not": true, "but": true, "you": true,
	"your": true, "into": true, "how": true, "why": true, "what": true,
	"new": true, "more": true, "can": true, "will": true, "all": true,
	"out": true, "his": true, "her": true, "their": true, "they": true,
	"them": true, "about": true, "after": true, "over": true, "when": true,
	"who": true, "which": true, "one": true, "two": true, "use": true,
	"using": true, "get": true, "gets": true, "than": true, "now": true,
}

// titleTokens splits a title into comparable tokens: Latin words of three or
// more letters, and character bigrams for CJK runs.
func titleTokens(title string) []string {
	var out []string
	var latin strings.Builder
	var cjk []rune

	flushLatin := func() {
		word := latin.String()
		latin.Reset()
		if len(word) >= 3 && !latinStopWords[word] {
			out = append(out, word)
		}
	}
	flushCJK := func() {
		switch len(cjk) {
		case 0:
		case 1:
			out = append(out, string(cjk[0]))
		default:
			for i := 0; i+1 < len(cjk); i++ {
				out = append(out, string(cjk[i:i+2]))
			}
		}
		cjk = cjk[:0]
	}

	for _, r := range title {
		switch {
		case unicode.Is(unicode.Han, r):
			flushLatin()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			latin.WriteRune(unicode.ToLower(r))
		default:
			flushLatin()
			flushCJK()
		}
	}
	flushLatin()
	flushCJK()

	seen := make(map[string]bool, len(out))
	unique := out[:0]
	for _, t := range out {
		if seen[t] {
			continue
		}
		seen[t] = true
		unique = append(unique, t)
	}
	return unique
}

// jaccard returns the overlap ratio of two token sets.
func jaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	small, large := a, b
	if len(small) > len(large) {
		small, large = large, small
	}
	set := make(map[string]bool, len(large))
	for _, t := range large {
		set[t] = true
	}
	hits := 0
	for _, t := range small {
		if set[t] {
			hits++
		}
	}
	return float64(hits) / float64(len(a)+len(b)-hits)
}

// sameStory is the clustering rule. Two titles describe one story either when
// they share most of their distinctive tokens, or when one title is almost
// entirely contained in the other ("Phoebe Bridgers announces MSG EP" inside
// "Phoebe Bridgers reveals new live at Madison Square Garden EP exclusively on
// Tidal").
func sameStory(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	small, large := len(a), len(b)
	if small > large {
		small, large = large, small
	}
	if float64(small) < 0.5*float64(large) {
		return false
	}
	j := jaccard(a, b)
	if j >= 0.55 {
		return true
	}
	// Containment: the shorter title is mostly re-used by the longer one.
	if small >= 4 {
		overlap := float64(j) * float64(len(a)+len(b)) / (1 + float64(j))
		if overlap/float64(small) >= 0.75 {
			return true
		}
	}
	return false
}

// candidate is one item plus the bookkeeping ranking needs.
type candidate struct {
	item   Item
	src    Source
	ts     time.Time
	tokens []string
}

// cluster is a group of candidates describing the same story.
type cluster struct {
	members []*candidate
	rep     *candidate
}

// finalizeCluster folds members into one digest item: the representative is
// the most authoritative report (tier first, then the earliest report), and the
// distinct feed count becomes the coverage signal.
func finalizeCluster(c *cluster) Item {
	rep := c.members[0]
	for _, m := range c.members[1:] {
		switch {
		case tierWeight(m.src.Tier) > tierWeight(rep.src.Tier):
			rep = m
		case tierWeight(m.src.Tier) == tierWeight(rep.src.Tier) && m.ts.Before(rep.ts):
			rep = m
		}
	}
	c.rep = rep

	feeds := make(map[string]bool, len(c.members))
	others := make([]string, 0, len(c.members))
	for _, m := range c.members {
		if feeds[m.src.ID] {
			continue
		}
		feeds[m.src.ID] = true
		if m.src.Name != rep.src.Name {
			others = append(others, m.src.Name)
		}
	}
	sort.Strings(others)

	item := rep.item
	item.SourceCount = len(feeds)
	item.Sources = others
	return item
}

// clusterPool groups candidates that describe the same story. Members are
// scanned newest first so the first report of a story anchors its cluster.
func clusterPool(cands []*candidate) []*cluster {
	sorted := make([]*candidate, len(cands))
	copy(sorted, cands)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].ts.Equal(sorted[j].ts) {
			return sorted[i].ts.After(sorted[j].ts)
		}
		return sorted[i].item.URL < sorted[j].item.URL
	})

	clusters := make([]*cluster, 0, len(sorted))
	for _, cand := range sorted {
		joined := false
		for _, cl := range clusters {
			if sameStory(cand.tokens, cl.members[0].tokens) {
				cl.members = append(cl.members, cand)
				joined = true
				break
			}
		}
		if !joined {
			clusters = append(clusters, &cluster{members: []*candidate{cand}})
		}
	}
	return clusters
}

// heat ranks one story. It is deliberately simple and explainable: a fresh
// first-party report outranks an old media rewrite, and every extra
// independent source covering the same story adds a fixed amount. Papers age
// far more slowly than news, because a month-old journal article is still new,
// and citations add a small standing signal.
func heat(item Item, src Source, ts, now time.Time) float64 {
	ageHours := now.Sub(ts).Hours()
	if ageHours < 0 {
		ageHours = 0
	}
	maxAge := 14 * 24.0
	if item.Kind == KindPaper {
		maxAge = 120 * 24.0
	}
	if ageHours > maxAge {
		ageHours = maxAge
	}

	var score float64
	if item.Kind == KindPaper {
		score = math.Exp(-ageHours/720.0) * 8 * tierWeight(src.Tier)
		if item.Citations > 0 {
			score += 1.2 * math.Log10(float64(item.Citations)+1)
		}
	} else {
		// Half-life of roughly 25 hours: yesterday's news still shows, last
		// week's does not crowd out today's.
		score = math.Exp(-ageHours/36.0) * 10 * tierWeight(src.Tier)
	}
	if item.SourceCount > 1 {
		score += 4 * math.Log2(float64(item.SourceCount))
	}
	hits := keywordHits(item.Group, item.Title+" "+item.Summary)
	if hits > 3 {
		hits = 3
	}
	weight := 0.5
	if item.Kind == KindPaper {
		weight = 0.25
	}
	score += float64(hits) * weight
	return math.Round(score*100) / 100
}
