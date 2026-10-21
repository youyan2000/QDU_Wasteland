package toolweb

import (
	"io/fs"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBookmarkCollections(t *testing.T) {
	tests := []struct {
		path, title string
		count       int
		items       []Bookmark
	}{
		{"/tool/information/", "随笔散文", len(bookmarks.Magazine), bookmarks.Magazine},
		{"/tool/toy/", "趣味角落", len(bookmarks.Toy), bookmarks.Toy},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			w := httptest.NewRecorder()
			New(Options{}).ServeHTTP(w, httptest.NewRequest("GET", tt.path, nil))
			if w.Code != 200 {
				t.Fatalf("status %d", w.Code)
			}
			if !strings.Contains(w.Body.String(), tt.title) {
				t.Fatal("page title missing")
			}
			if got := strings.Count(w.Body.String(), `class="collection-card"`); got != tt.count {
				t.Fatalf("cards %d, want %d", got, tt.count)
			}
			if strings.Contains(w.Body.String(), "localhost") {
				t.Fatal("local-only bookmark leaked into public collection")
			}
			if !strings.Contains(w.Body.String(), `rel="noopener noreferrer"`) {
				t.Fatal("external links are missing safe rel attributes")
			}
			if !strings.Contains(w.Body.String(), `class="collection-cover-image"`) {
				t.Fatal("large local cover image missing")
			}
			if strings.Count(w.Body.String(), `/tool/_assets/covers/`) != tt.count {
				t.Fatal("per-card cover URLs missing")
			}
			if !strings.Contains(w.Body.String(), `data-bookmark-id=`) {
				t.Fatal("bookmark randomization key missing")
			}
			for _, item := range tt.items {
				if !strings.Contains(w.Body.String(), item.Title) && !strings.Contains(w.Body.String(), strings.ReplaceAll(item.Title, "&", "&amp;")) {
					t.Errorf("missing title %q", item.Title)
				}
				if !strings.Contains(w.Body.String(), item.URL) && !strings.Contains(w.Body.String(), strings.ReplaceAll(item.URL, "&", "&amp;")) {
					t.Errorf("missing URL %q", item.URL)
				}
			}
			for _, item := range bookmarks.Magazine {
				u, err := url.Parse(item.URL)
				if err != nil || u.Query().Has("utm_source") || u.Query().Has("srsltid") {
					t.Errorf("tracking parameter remains in %q", item.URL)
				}
			}
			if strings.Contains(w.Body.String(), "趣味随笔") {
				t.Error("old collection title remains")
			}
			if strings.Contains(w.Body.String(), "6pen.art") || strings.Contains(w.Body.String(), "tool.520101.com") {
				t.Error("user-removed bookmark remains")
			}
			if strings.Contains(w.Body.String(), `class="collection-cover-title"`) {
				t.Error("duplicate cover title remains")
			}
			if strings.Count(w.Body.String(), `<h2>`) != tt.count {
				t.Error("each card must have exactly one visible title")
			}
		})
	}
}

func TestBookmarkCoverKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, items := range [][]Bookmark{bookmarks.Magazine, bookmarks.Toy} {
		for _, item := range items {
			if item.Image == "" {
				t.Errorf("photo image missing for %q", item.Title)
				continue
			}
			if seen[item.Image] {
				t.Errorf("photo image %q is reused; each site should have its own image", item.Image)
			}
			seen[item.Image] = true
			if _, err := fs.Stat(assets, "static/covers/photos/"+item.Image); err != nil {
				t.Errorf("photo image %q for %q missing: %v", item.Image, item.Title, err)
			}
		}
	}
	if len(seen) != 82 {
		t.Errorf("unique local photo covers = %d, want 82", len(seen))
	}
	for _, item := range bookmarks.Toy {
		if strings.Contains(item.URL, "6pen.art") {
			t.Error("6pen Art should be removed")
		}
	}
	for _, item := range bookmarks.Magazine {
		if strings.Contains(item.URL, "youtube.com/") {
			t.Errorf("video channel should not be in essays: %s", item.URL)
		}
		if item.Title == "电子线路在线计算" {
			t.Error("circuit calculator should not be in essays")
		}
		if item.Title == "PDF 编辑器" {
			t.Error("PDF editor should not be in essays")
		}
	}
}

func TestBookmarkCanonicalRoutes(t *testing.T) {
	for _, path := range []string{"/tool/information", "/tool/toy"} {
		w := httptest.NewRecorder()
		New(Options{}).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 308 {
			t.Fatalf("%s: status %d, want 308", path, w.Code)
		}
	}
}
