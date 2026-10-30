package hotdigest

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveFeeds fetches the real subscription list. It is skipped by default so
// ordinary test runs stay offline; enable with HOTDIGEST_LIVE=1.
func TestLiveFeeds(t *testing.T) {
	if os.Getenv("HOTDIGEST_LIVE") != "1" {
		t.Skip("set HOTDIGEST_LIVE=1 to fetch the real feeds")
	}
	store := NewStore(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	d, err := store.Refresh(ctx, true)
	if err != nil {
		t.Fatalf("live refresh: %v", err)
	}
	if len(d.Items) == 0 {
		t.Fatal("no items returned")
	}
	if len(d.Items) > maxPerDigest {
		t.Fatalf("items = %d, want <= %d", len(d.Items), maxPerDigest)
	}

	counts := make(map[string]int)
	papers := make(map[string]int)
	for _, it := range d.Items {
		counts[it.Group]++
		if it.Kind == KindPaper {
			papers[it.Group]++
		}
		if it.Title == "" || it.URL == "" || it.PublishedAt == "" || it.Source == "" {
			t.Fatalf("incomplete item: %+v", it)
		}
		if it.Heat <= 0 {
			t.Fatalf("item without heat: %+v", it)
		}
		if it.Kind != KindNews && it.Kind != KindPaper {
			t.Fatalf("item without a kind: %+v", it)
		}
	}
	for _, group := range GroupOrder {
		spec := groupSpecs[group]
		if counts[group] == 0 {
			t.Errorf("section %q is empty (failed sources: %v)", group, d.Failed)
			continue
		}
		if counts[group] > spec.max {
			t.Errorf("section %q has %d items, max is %d", group, counts[group], spec.max)
		}
		t.Logf("%-9s %2d items (%d papers) target=%d", group, counts[group], papers[group], spec.target)
	}
	t.Logf("date=%s items=%d sources=%d failed=%d", d.Date, len(d.Items), d.SourceTotal, d.SourceFailed)
	for _, it := range d.Items {
		if it.Kind == KindPaper {
			t.Logf("  [%s/%s] %.2f %s | %s | %s", it.Group, it.Kind, it.Heat, it.Source, it.Title, it.URL)
		}
	}
}
