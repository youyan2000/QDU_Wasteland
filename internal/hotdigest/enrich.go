package hotdigest

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"
)

// Enricher curates a field with an OpenAI-compatible chat model: it picks the
// stories worth showing and writes their titles and summaries. It never changes
// the language family of a story — Chinese stays Chinese, everything else is
// written in English. When no key is configured the digest runs on rules alone.
type Enricher struct {
	APIURL string
	APIKey string
	Model  string
	Client *http.Client
	// RetryDelay is the first pause between retries; zero means 4 seconds.
	RetryDelay time.Duration
}

// NewEnricher returns an enricher, or nil when no API key is configured.
// AI_API_URL and AI_MODEL follow the site's existing AI review settings.
func NewEnricher(apiKey string) *Enricher {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil
	}
	apiURL := strings.TrimSpace(os.Getenv("AI_API_URL"))
	if apiURL == "" {
		apiURL = "https://api.openai.com/v1/chat/completions"
	}
	parsedURL, err := url.Parse(apiURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.User != nil || parsedURL.Hostname() == "" || (parsedURL.Port() != "" && parsedURL.Port() != "443") {
		return nil
	}
	host := strings.ToLower(strings.TrimSuffix(parsedURL.Hostname(), "."))
	allowed := map[string]bool{"api.openai.com": true, "api.deepseek.com": true}
	for _, entry := range strings.Split(os.Getenv("AI_ALLOWED_HOSTS"), ",") {
		entry = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(entry, ".")))
		if entry != "" {
			allowed[entry] = true
		}
	}
	if !allowed[host] {
		return nil
	}
	apiURL = parsedURL.String()
	model := strings.TrimSpace(os.Getenv("AI_MODEL"))
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &Enricher{
		APIURL: apiURL,
		APIKey: apiKey,
		Model:  model,
		Client: &http.Client{
			Timeout: 120 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// targetIsChinese reports which language family a story must be displayed in.
func targetIsChinese(item Item) bool {
	if item.Lang != "" {
		return item.Lang == "zh"
	}
	han, other := 0, 0
	for _, r := range item.Title {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.IsLetter(r):
			other++
		}
	}
	return han > 0 && han >= other
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// englishStopWords and foreignStopWords drive looksEnglish. A title that only
// uses Latin letters can still be French or German, so the check looks for
// accented letters and for function words from the languages we follow.
var englishStopWords = []string{
	"the", "of", "and", "in", "to", "is", "for", "with", "on", "that",
	"from", "by", "as", "at", "an", "are", "be", "how", "why", "what",
	"about", "into", "over", "after", "between", "not", "its",
}

var foreignStopWords = []string{
	// French
	"et", "les", "des", "du", "une", "un", "dans", "pour", "avec", "sur",
	"par", "est", "aux", "leur", "nous", "vous", "comme", "plus", "sans",
	// German
	"der", "die", "das", "und", "von", "zu", "mit", "für", "über", "ist",
	"eine", "einen", "dem", "den", "nicht", "als", "zur", "zum", "bei",
	// Spanish / Portuguese / Italian
	"el", "los", "las", "una", "para", "por", "con", "del", "que", "una",
	"della", "dello", "degli", "come", "anche", "nella", "nello",
	// Dutch
	"het", "een", "van", "voor", "met",
}

// looksEnglish reports whether a title or summary already reads as English.
// It is deliberately conservative: an unrecognised Latin string counts as
// English so that legitimate titles are never rejected by accident.
func looksEnglish(s string) bool {
	if strings.TrimSpace(s) == "" {
		return true
	}
	hasEnglish := containsWord(s, englishStopWords)
	if hasEnglish {
		return true
	}
	for _, r := range s {
		if unicode.IsLetter(r) && r > unicode.MaxASCII {
			// Accented Latin, Cyrillic, CJK…: not English.
			return false
		}
	}
	if containsWord(s, foreignStopWords) {
		return false
	}
	return true
}

// containsWord matches whole words, ignoring case and surrounding punctuation.
func containsWord(s string, words []string) bool {
	lower := strings.ToLower(s)
	for _, w := range words {
		idx := 0
		for {
			p := strings.Index(lower[idx:], w)
			if p < 0 {
				break
			}
			p += idx
			end := p + len(w)
			startOK := p == 0 || !isWordByte(lower[p-1])
			endOK := end == len(lower) || !isWordByte(lower[end])
			if startOK && endOK {
				return true
			}
			idx = p + 1
		}
	}
	return false
}

// oneLine collapses whitespace, drops wrapping quotes and bounds the length.
func oneLine(s string, maxRunes int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	s = strings.Trim(s, "\"“”'‘’「」《》")
	return truncateRunes(strings.TrimSpace(s), maxRunes)
}
