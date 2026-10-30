package hotdigest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// crossrefRows bounds one journal query; the daily cap is applied later by the
// per-source limit in selection.
const crossrefRows = 25

// crossrefAPI is the journal metadata endpoint (no key, no registration).
const crossrefAPI = "https://api.crossref.org/journals"

// Crossref answers 429 as soon as a handful of requests overlap, so journal
// queries are serialised with a minimum interval instead of running in the
// feed pool. One run issues roughly forty of them.
const (
	crossrefInterval = 1200 * time.Millisecond
	crossrefRetryGap = 5 * time.Second
)

type crossrefDate struct {
	DateParts [][]int `json:"date-parts"`
}

type crossrefWork struct {
	DOI       string   `json:"DOI"`
	Title     []string `json:"title"`
	Abstract  string   `json:"abstract"`
	Container []string `json:"container-title"`
	Citations int      `json:"is-referenced-by-count"`
	Type      string   `json:"type"`

	PublishedOnline crossrefDate `json:"published-online"`
	PublishedPrint  crossrefDate `json:"published-print"`
	Issued          crossrefDate `json:"issued"`
	Created         struct {
		DateTime string `json:"date-time"`
	} `json:"created"`
}

type crossrefResponse struct {
	Message struct {
		Items []crossrefWork `json:"items"`
	} `json:"message"`
}

// waitCrossref spaces journal queries so Crossref does not answer 429.
func (s *Store) waitCrossref(ctx context.Context) error {
	s.crossrefMu.Lock()
	defer s.crossrefMu.Unlock()
	if !s.crossrefAt.IsZero() {
		if wait := crossrefInterval - time.Since(s.crossrefAt); wait > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
	}
	s.crossrefAt = time.Now()
	return nil
}

// fetchJournal reads the newest works of one journal and turns them into paper
// items. Crossref carries no abstract for every publisher (IEEE deposits none),
// so a title-only entry is expected and still useful.
func (s *Store) fetchJournal(ctx context.Context, src Source, since time.Time) ([]rawItem, error) {
	endpoint := crossrefAPI + "/" + url.PathEscape(strings.TrimSpace(src.ISSN)) + "/works" +
		"?rows=" + fmt.Sprint(crossrefRows) +
		"&sort=created&order=desc" +
		"&filter=from-created-date:" + since.Format("2006-01-02") +
		"&select=DOI,title,abstract,container-title,type,published-online,published-print,issued,created,is-referenced-by-count"

	var body []byte
	for attempt := 0; ; attempt++ {
		if err := s.waitCrossref(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := s.client().Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			if attempt == 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(crossrefRetryGap):
				}
				continue
			}
			return nil, fmt.Errorf("crossref http %d", resp.StatusCode)
		}
		if resp.StatusCode != http.StatusOK {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			return nil, fmt.Errorf("crossref http %d", resp.StatusCode)
		}
		body, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		break
	}

	var parsed crossrefResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	now := s.now()
	items := make([]rawItem, 0, len(parsed.Message.Items))
	for _, work := range parsed.Message.Items {
		// Book reviews, editorials and errata are deposited as works but are
		// not what a "read this paper" list is for: French and German journals
		// deposit dozens of book-review citations per issue.
		if !articleTypes[strings.ToLower(strings.TrimSpace(work.Type))] {
			continue
		}
		title := strings.TrimSpace(firstNonEmpty(work.Title...))
		doi := strings.TrimSpace(work.DOI)
		if title == "" || doi == "" {
			continue
		}
		published := crossrefPublishedAt(work, now)
		if published.IsZero() {
			continue
		}
		// A paper published years ago and re-deposited today is not new work.
		if issued := crossrefDateValue(work.Issued); !issued.IsZero() && issued.Before(now.AddDate(-2, 0, 0)) {
			continue
		}
		summary := plainText(work.Abstract, 0)
		summary = strings.TrimSpace(strings.TrimPrefix(summary, "Abstract"))
		summary = strings.TrimSpace(strings.TrimPrefix(summary, "ABSTRACT"))
		items = append(items, rawItem{
			// Crossref titles sometimes carry inline markup such as <sup>1</sup>.
			Title:     truncateRunes(plainText(title, 0), 320),
			Link:      "https://doi.org/" + doi,
			Summary:   truncateRunes(summary, 320),
			Published: published,
			GUID:      doi,
			Kind:      KindPaper,
			Venue:     firstNonEmpty(work.Container...),
			Citations: work.Citations,
			Issue:     crossrefIssueLabel(work),
		})
	}
	// A quiet journal is not a broken source: it simply has no new works.
	return items, nil
}

// articleTypes are the Crossref work types worth reading. Book reviews,
// editorials, errata and similar records are filtered out.
var articleTypes = map[string]bool{
	"journal-article":     true,
	"proceedings-article": true,
	"posted-content":      true,
	"book-chapter":        false,
}

// crossrefPublishedAt picks the most trustworthy date. IEEE deposits print
// issue months that are scheduled ahead of time, so the deposit timestamp is
// the only field that answers "when did this become available".
func crossrefPublishedAt(work crossrefWork, now time.Time) time.Time {
	if raw := strings.TrimSpace(work.Created.DateTime); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil && !t.IsZero() {
			return t
		}
	}
	if t := crossrefDateValue(work.PublishedOnline); !t.IsZero() {
		return t
	}
	if t := crossrefDateValue(work.Issued); !t.IsZero() && !t.After(now.AddDate(0, 0, 120)) {
		return t
	}
	return time.Time{}
}

// crossrefIssueLabel renders the publication date the page shows for a paper
// ("2026-11" for a monthly issue). The deposit timestamp is used for ranking,
// but it is not the issue date and would make every paper look brand new.
func crossrefIssueLabel(work crossrefWork) string {
	for _, d := range []crossrefDate{work.PublishedPrint, work.Issued, work.PublishedOnline} {
		if len(d.DateParts) == 0 || len(d.DateParts[0]) == 0 {
			continue
		}
		parts := d.DateParts[0]
		t := crossrefDateValue(d)
		if t.IsZero() {
			continue
		}
		switch {
		case len(parts) > 2 && parts[2] >= 1 && parts[2] <= 31:
			return t.Format("2006-01-02")
		case len(parts) > 1:
			return t.Format("2006-01")
		default:
			return t.Format("2006")
		}
	}
	return ""
}

func crossrefDateValue(d crossrefDate) time.Time {
	if len(d.DateParts) == 0 || len(d.DateParts[0]) < 1 {
		return time.Time{}
	}
	parts := d.DateParts[0]
	year := parts[0]
	if year < 1900 {
		return time.Time{}
	}
	month := time.January
	if len(parts) > 1 && parts[1] >= 1 && parts[1] <= 12 {
		month = time.Month(parts[1])
	}
	day := 1
	if len(parts) > 2 && parts[2] >= 1 && parts[2] <= 31 {
		day = parts[2]
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
