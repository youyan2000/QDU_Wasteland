package tools

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":     {Data: []byte("<html>home</html>")},
		"app.js":         {Data: []byte("console.log(1)")},
		"style.css":      {Data: []byte("body{}")},
		"secret.go":      {Data: []byte("package main")},
		"sub/index.html": {Data: []byte("<html>sub</html>")},
		"empty.txt":      {Data: []byte("")},
	}
}

func TestStaticServesIndexForRoot(t *testing.T) {
	h := Static{FS: testFS()}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "home") {
		t.Fatalf("应返回 index.html，got %q", rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("默认缓存策略应为 no-store，got %q", cc)
	}
}

func TestStaticServesNestedIndex(t *testing.T) {
	h := Static{FS: testFS()}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sub/", nil))

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "sub") {
		t.Fatalf("want sub/index.html, got %d %q", rec.Code, rec.Body.String())
	}
}

func TestStaticSetsContentType(t *testing.T) {
	h := Static{FS: testFS()}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") && !strings.HasPrefix(ct, "application/javascript") {
		t.Fatalf("Content-Type 不符: %q", ct)
	}
}

func TestStaticRejectsDisallowedExtension(t *testing.T) {
	h := Static{FS: testFS()}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/secret.go", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("非白名单扩展名应 404，got %d", rec.Code)
	}
}

func TestStaticRejectsTraversal(t *testing.T) {
	h := Static{FS: testFS()}
	for _, p := range []string{"/../go.mod", "/sub/../../go.mod", "/sub\\index.html", "/./."} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("path %q 应被拒绝，got %d", p, rec.Code)
		}
	}
}

func TestStaticMissingFileIs404(t *testing.T) {
	h := Static{FS: testFS()}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope.js", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestStaticRejectsNonGet(t *testing.T) {
	h := Static{FS: testFS()}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/app.js", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}

func TestStaticNilFSIs404(t *testing.T) {
	h := Static{}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestStaticCustomCacheControl(t *testing.T) {
	h := Static{FS: testFS(), CacheControl: "public, max-age=60"}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/style.css", nil))

	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Fatalf("want custom cache, got %q", cc)
	}
}
