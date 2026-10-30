package hotdigest

import (
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// maxFeedBytes bounds how much of one feed body is read.
	maxFeedBytes = 4 << 20
	// maxEntriesPerFeed keeps a single feed from dominating the candidate pool.
	maxEntriesPerFeed = 120
)

// The two kinds of item a digest mixes: press coverage and journal papers.
const (
	KindNews  = "news"
	KindPaper = "paper"
)

// rawItem is one entry parsed out of an RSS or Atom feed.
type rawItem struct {
	Title     string
	Link      string
	Summary   string
	Published time.Time
	GUID      string
	// Kind is KindNews for feed entries and KindPaper for journal works.
	Kind      string
	Venue     string
	Citations int
	// Issue is the journal's publication date label ("2026-11"), shown instead
	// of a relative time because the deposit timestamp is not the issue date.
	Issue string
}

// feedLink covers both shapes of <link>: Atom carries the URL in href, RSS 2.0
// carries it as element text.
type feedLink struct {
	Rel   string `xml:"rel,attr"`
	Href  string `xml:"href,attr"`
	Value string `xml:",chardata"`
}

type feedEntry struct {
	Title     string     `xml:"title"`
	Links     []feedLink `xml:"link"`
	Desc      string     `xml:"description"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"encoded"`
	PubDate   string     `xml:"pubDate"`
	Date      string     `xml:"date"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	GUID      string     `xml:"guid"`
	ID        string     `xml:"id"`
}

// feedDoc accepts RSS 2.0 (<rss><channel><item>), RSS 1.0 (<rdf:RDF><item>)
// and Atom (<feed><entry>) in one pass; the entry list that actually has
// content decides which shape the feed used.
type feedDoc struct {
	XMLName xml.Name
	Channel struct {
		Title string      `xml:"title"`
		Items []feedEntry `xml:"item"`
	} `xml:"channel"`
	Items   []feedEntry `xml:"item"`
	Title   string      `xml:"title"`
	Entries []feedEntry `xml:"entry"`
}

func (e feedEntry) link() string {
	for _, l := range e.Links {
		if l.Href == "" {
			continue
		}
		if l.Rel == "" || l.Rel == "alternate" {
			return strings.TrimSpace(l.Href)
		}
	}
	for _, l := range e.Links {
		if v := strings.TrimSpace(l.Value); v != "" {
			return v
		}
	}
	return ""
}

func (e feedEntry) publishedAt() time.Time {
	return parseFeedTime(coalesce(e.PubDate, e.Published, e.Date, e.Updated))
}

func (e feedEntry) body() string {
	return coalesce(e.Desc, e.Summary, e.Content)
}

func coalesce(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// parseFeed reads one RSS or Atom document into raw items. Malformed entries
// are skipped rather than failing the whole feed.
func parseFeed(r io.Reader) ([]rawItem, error) {
	dec := xml.NewDecoder(io.LimitReader(r, maxFeedBytes))
	dec.Strict = false
	dec.CharsetReader = charsetReader
	var doc feedDoc
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	entries := doc.Entries
	if len(entries) == 0 {
		entries = doc.Channel.Items
	}
	if len(entries) == 0 {
		entries = doc.Items
	}
	items := make([]rawItem, 0, len(entries))
	for _, e := range entries {
		if len(items) >= maxEntriesPerFeed {
			break
		}
		item := rawItem{
			Title:     truncateRunes(plainText(e.Title, 0), 300),
			Link:      e.link(),
			Summary:   truncateRunes(stripFeedBoilerplate(plainText(e.body(), 0)), 280),
			GUID:      coalesce(e.GUID, e.ID),
			Published: e.publishedAt(),
			Kind:      KindNews,
		}
		if item.Title == "" || item.Link == "" {
			continue
		}
		if item.Summary == item.Title {
			item.Summary = ""
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("feed has no usable entries")
	}
	return items, nil
}

// boilerplateTails are the trailing fragments feeds append to every excerpt.
var boilerplateTails = []string{
	"read more", "continue reading", "read the full story", "read the full article",
	"view this email in your browser",
}

// stripFeedBoilerplate removes the WordPress-style suffix ("The post X appeared
// first on Y") and the trailing "Read more" links that would otherwise appear
// inside every summary.
func stripFeedBoilerplate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	lower := strings.ToLower(s)
	if i := strings.LastIndex(lower, "appeared first on"); i > 0 {
		cut := strings.LastIndex(lower[:i], "the post")
		if cut < 0 {
			// Without the "The post" lead-in, drop the sentence that carries it.
			cut = strings.LastIndex(lower[:i], ". ")
			if cut < 0 {
				cut = i
			}
		}
		s = strings.TrimSpace(s[:cut])
		if s == "" {
			return ""
		}
		lower = strings.ToLower(s)
	}
	// Turn a trailing "[…]" / "[...]" into an ellipsis: it marks where the feed
	// cut the excerpt, so dropping it silently would read as a complete thought.
	for _, tail := range []string{"[…]", "[...]", "(…)"} {
		if !strings.HasSuffix(s, tail) {
			continue
		}
		s = strings.TrimSpace(strings.TrimSuffix(s, tail))
		if !strings.HasSuffix(s, "…") {
			s += "…"
		}
		lower = strings.ToLower(s)
		break
	}
	for _, tail := range boilerplateTails {
		if !strings.HasSuffix(lower, tail) {
			continue
		}
		cut := s[:len(s)-len(tail)]
		if strings.HasSuffix(strings.ToLower(strings.TrimSpace(cut)), "the post") {
			cut = strings.TrimSpace(cut)
			cut = cut[:len(cut)-len("the post")]
		}
		// Keep a sentence-ending period; only drop separators and spaces.
		s = strings.TrimSpace(strings.TrimRight(cut, " ·-—–»"))
		break
	}
	return s
}

// charsetReader supports UTF-8 (the overwhelming majority of feeds) and
// Latin-1 fallbacks. Legacy CJK feeds must be served as UTF-8; an unsupported
// charset is reported per feed and does not affect the other sources.
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(charset)) {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return input, nil
	case "iso-8859-1", "latin1", "latin-1", "windows-1252":
		raw, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		for _, c := range raw {
			b.WriteRune(rune(c))
		}
		return strings.NewReader(b.String()), nil
	default:
		return nil, fmt.Errorf("unsupported feed charset %q", charset)
	}
}

var feedTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	time.RFC1123Z,
	time.RFC1123,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"2 Jan 2006 15:04:05 -0700",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// parseFeedTime tries the date layouts used by RSS 2.0, Atom and the common
// non-standard variants. A zero time means "no trusted publication date".
func parseFeedTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range feedTimeLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

var (
	tagRE   = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRE = regexp.MustCompile(`\s+`)
)

// plainText turns an HTML/markup fragment into one line of text.
func plainText(raw string, maxRunes int) string {
	if raw == "" {
		return ""
	}
	// Strip tags before unescaping so "&lt;b&gt;" stays visible text.
	s := tagRE.ReplaceAllString(raw, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.TrimSpace(spaceRE.ReplaceAllString(s, " "))
	return truncateRunes(s, maxRunes)
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:max])) + "…"
}

// trackingParams are stripped so the same article arriving through two links
// is recognised as one item.
var trackingParams = map[string]bool{
	"fbclid": true, "gclid": true, "ref": true, "referrer": true,
	"source": true, "spm": true, "from": true, "share_source": true,
	"share_medium": true, "share_plat": true, "share_tag": true,
	"vd_source": true, "unique_k": true, "utm": true, "mc_cid": true,
	"mc_eid": true, "cmpid": true, "ocid": true,
}

// canonicalURL normalises an article URL so duplicates collapse onto one key.
// It returns "" for anything that is not an absolute http(s) URL.
func canonicalURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	if u.Host == "" {
		return ""
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	if u.RawQuery != "" {
		q := u.Query()
		for key := range q {
			if trackingParams[key] || strings.HasPrefix(key, "utm_") || strings.HasPrefix(key, "pk_") {
				q.Del(key)
			}
		}
		u.RawQuery = q.Encode()
	}
	if u.Path != "/" {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String()
}

// stripSiteSuffix removes a trailing " - Site Name" / " | Site Name" that
// feeds commonly append to every title.
func stripSiteSuffix(title, name string) string {
	name = strings.TrimSpace(name)
	if name == "" || title == "" {
		return title
	}
	lowerTitle := strings.ToLower(title)
	lowerSuffix := strings.ToLower(name)
	for _, sep := range []string{" - ", " | ", " – ", " — ", " · ", " :: ", " — "} {
		if strings.HasSuffix(lowerTitle, sep+lowerSuffix) {
			cut := len(title) - len(sep) - len(name)
			if cut > 0 {
				return strings.TrimSpace(title[:cut])
			}
		}
	}
	return title
}
