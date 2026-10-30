package hotdigest

import (
	"strings"
	"testing"
	"time"
)

func TestParseRSS2(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Example</title>
<item>
  <title>Hello &amp; goodbye</title>
  <link>https://example.com/a?utm_source=newsletter</link>
  <description><![CDATA[<p>Body <b>text</b> here</p>]]></description>
  <pubDate>Thu, 30 Oct 2026 09:00:00 +0800</pubDate>
  <guid>https://example.com/a</guid>
</item>
<item><title>No date</title><link>https://example.com/b</link></item>
</channel></rss>`

	items, err := parseFeed(strings.NewReader(feed))
	if err != nil {
		t.Fatalf("parseFeed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].Title != "Hello & goodbye" {
		t.Errorf("title = %q", items[0].Title)
	}
	if items[0].Link != "https://example.com/a?utm_source=newsletter" {
		t.Errorf("link = %q", items[0].Link)
	}
	if items[0].Summary != "Body text here" {
		t.Errorf("summary = %q", items[0].Summary)
	}
	if items[0].Published.IsZero() {
		t.Error("pubDate was not parsed")
	}
	if !items[1].Published.IsZero() {
		t.Error("missing pubDate should stay zero")
	}
}

func TestParseAtom(t *testing.T) {
	feed := `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Simon Willison</title>
  <entry>
    <title>Notes on a model</title>
    <link rel="self" href="https://example.com/self"/>
    <link rel="alternate" href="https://example.com/post"/>
    <summary>Short summary</summary>
    <updated>2026-10-29T10:00:00Z</updated>
  </entry>
</feed>`

	items, err := parseFeed(strings.NewReader(feed))
	if err != nil {
		t.Fatalf("parseFeed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].Link != "https://example.com/post" {
		t.Errorf("link = %q, want the alternate link", items[0].Link)
	}
	if items[0].Summary != "Short summary" {
		t.Errorf("summary = %q", items[0].Summary)
	}
	if items[0].Published.IsZero() {
		t.Error("updated was not used as the publication date")
	}
}

func TestParseFeedRejectsEmpty(t *testing.T) {
	if _, err := parseFeed(strings.NewReader("<rss><channel></channel></rss>")); err == nil {
		t.Fatal("expected an error for a feed with no entries")
	}
}

func TestParseFeedTimeLayouts(t *testing.T) {
	cases := map[string]bool{
		"Thu, 30 Oct 2026 09:00:00 +0800": true,
		"2026-10-30T09:00:00Z":            true,
		"2026-10-30":                      true,
		"not a date":                      false,
		"":                                false,
	}
	for raw, ok := range cases {
		got := parseFeedTime(raw)
		if ok != !got.IsZero() {
			t.Errorf("parseFeedTime(%q) zero = %v, want parsed = %v", raw, got.IsZero(), ok)
		}
	}
}

func TestCanonicalURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://Example.com/Post/?utm_source=x&utm_medium=y", "https://example.com/Post"},
		{"https://example.com/a#section", "https://example.com/a"},
		{"https://example.com/a?ref=hn&id=7", "https://example.com/a?id=7"},
		{"https://example.com/", "https://example.com/"},
		{"ftp://example.com/a", ""},
		{"/relative/path", ""},
		{"javascript:alert(1)", ""},
	}
	for _, c := range cases {
		if got := canonicalURL(c.in); got != c.want {
			t.Errorf("canonicalURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPlainText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<p>Hello <b>world</b></p>", "Hello world"},
		{"A &amp; B&nbsp;C", "A & B C"},
		{"&lt;script&gt; is text", "<script> is text"},
	}
	for _, c := range cases {
		if got := plainText(c.in, 0); got != c.want {
			t.Errorf("plainText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := plainText("abcdef", 3); got != "abc…" {
		t.Errorf("truncate = %q", got)
	}
}

func TestStripSiteSuffix(t *testing.T) {
	if got := stripSiteSuffix("A new converter - Power Electronics News", "Power Electronics News"); got != "A new converter" {
		t.Errorf("suffix not stripped: %q", got)
	}
	if got := stripSiteSuffix("量子位：模型发布", "量子位"); got != "量子位：模型发布" {
		t.Errorf("unexpected change: %q", got)
	}
}

func TestStripFeedBoilerplate(t *testing.T) {
	cases := []struct{ in, want string }{
		{
			"GaN switches cut losses. The post GaN switches cut losses appeared first on Power Electronics News .",
			"GaN switches cut losses.",
		},
		{
			"Body text here […]",
			"Body text here…",
		},
		{
			"A short paragraph. Read more",
			"A short paragraph.",
		},
		{
			"Plain summary with no suffix",
			"Plain summary with no suffix",
		},
	}
	for _, c := range cases {
		if got := stripFeedBoilerplate(c.in); got != c.want {
			t.Errorf("stripFeedBoilerplate(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsBookkeeping(t *testing.T) {
	junk := []string{
		"Issue Information",
		"Table of Contents",
		"Editor\u2019s Note: Testing abductions from uncertain evidence",
		"Correction: A previous error",
		"Front Matter",
	}
	for _, title := range junk {
		if !isBookkeeping(title) {
			t.Errorf("%q should be filtered as issue furniture", title)
		}
	}
	keep := []string{
		"On latitude in attitude. Multidimensionalism and the right kind of reasons for belief",
		"No determinate facts, no objective oughts?",
		"Duty and permission for utilitarians",
	}
	for _, title := range keep {
		if isBookkeeping(title) {
			t.Errorf("%q is a real article and must be kept", title)
		}
	}
}

func TestKeywordBoundary(t *testing.T) {
	if !onTopic(GroupPower, "SiC inverter cuts switching losses") {
		t.Error("expected an on-topic power item")
	}
	if onTopic(GroupPower, "Organ music festival announces lineup") {
		t.Error("'gan' must not fire inside 'organ'")
	}
	if onTopic(GroupPower, "The music industry in 2026") {
		t.Error("'sic' must not fire inside 'music'")
	}
	if !onTopic(GroupEmbedded, "New STM32 board with Zephyr RTOS support") {
		t.Error("expected an on-topic embedded item")
	}
	if onTopic(GroupPhilosophy, "Quarterly earnings call transcript") {
		t.Error("earnings news is not philosophy")
	}
}

func TestSameStoryClustering(t *testing.T) {
	a := titleTokens("Open-weight model beats coding benchmarks")
	b := titleTokens("Researchers say open weight model beats coding benchmarks")
	c := titleTokens("A new soldering iron for hobbyists")
	if !sameStory(a, b) {
		t.Error("two reports of one story should cluster")
	}
	if sameStory(a, c) {
		t.Error("unrelated titles must not cluster")
	}
}

func TestHeatRanksFreshFirstPartyHigher(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	fresh := Item{Group: GroupAI, Title: "New open-weight model released"}
	old := Item{Group: GroupAI, Title: "New open-weight model released"}

	freshScore := heat(fresh, Source{Tier: "T1"}, now.Add(-2*time.Hour), now)
	oldScore := heat(old, Source{Tier: "T1"}, now.Add(-8*24*time.Hour), now)
	if freshScore <= oldScore {
		t.Fatalf("fresh %v should beat old %v", freshScore, oldScore)
	}

	mediaScore := heat(fresh, Source{Tier: "T2"}, now.Add(-2*time.Hour), now)
	if freshScore <= mediaScore {
		t.Fatalf("first-party %v should beat media %v", freshScore, mediaScore)
	}

	covered := fresh
	covered.SourceCount = 4
	if heat(covered, Source{Tier: "T1"}, now.Add(-2*time.Hour), now) <= freshScore {
		t.Fatal("wider coverage should raise heat")
	}
}
