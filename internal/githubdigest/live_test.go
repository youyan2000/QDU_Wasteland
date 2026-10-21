package githubdigest

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveGitHub hits the real GitHub search API. It is skipped by default so
// normal test runs stay offline; enable with DIGEST_LIVE=1.
func TestLiveGitHub(t *testing.T) {
	if os.Getenv("DIGEST_LIVE") != "1" {
		t.Skip("set DIGEST_LIVE=1 to query the real GitHub API")
	}
	store := NewStore(t.TempDir(), os.Getenv("GITHUB_TOKEN"))
	store.Delay = 2 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	d, err := store.Refresh(ctx, true)
	if err != nil {
		t.Fatalf("live refresh: %v", err)
	}
	if len(d.Items) == 0 {
		t.Fatal("no items returned")
	}
	for _, it := range d.Items {
		if it.FullName == "" || it.URL == "" {
			t.Fatalf("incomplete item: %+v", it)
		}
	}
	if len(d.Items) > 12 {
		t.Fatalf("items = %d, want <= 12", len(d.Items))
	}
	t.Logf("date=%s items=%d", d.Date, len(d.Items))
	for _, it := range d.Items[:min(5, len(d.Items))] {
		t.Logf("  %s ★%d %s", it.FullName, it.Stars, it.Description)
	}
}
