package toolweb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestToolRoutes(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nextjlc", "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"index.html": "<!doctype html><title>NeXTJLC</title>", "assets/main.js": "console.log('ok')", "assets/core.wasm": "\x00asm\x01\x00\x00\x00", "private.db": "DO NOT SERVE", "assets/main.js.map": "{}", ".secret.json": "{}"} {
		if err := os.WriteFile(filepath.Join(root, "nextjlc", filepath.FromSlash(name)), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	h := New(Options{Root: root, Demo: true})
	cases := []struct {
		method, path string
		status       int
		content      string
	}{
		{"GET", "/tool", 308, ""},
		{"GET", "/tool/", 200, "NeXTJLC"},
		{"GET", "/tool/_assets/tool-center.css", 200, "var(--magenta)"},
		{"GET", "/tool/_assets/tool-center.js", 200, "icon-fill"},
		{"GET", "/tool/nextjlc", 308, ""},
		{"GET", "/tool/nextjlc/", 200, "NeXTJLC"},
		{"HEAD", "/tool/nextjlc/", 200, ""},
		{"GET", "/tool/nextjlc/assets/main.js", 200, "console.log"},
		{"GET", "/tool/nextjlc/assets/core.wasm", 200, ""},
		{"GET", "/tool/nextjlc/assets/missing.wasm", 404, ""},
		{"GET", "/tool/nextjlc/assets/", 404, ""},
		{"GET", "/tool/unknown/", 404, ""},
		{"GET", "/tool/history/", 404, ""},
		{"GET", "/tool/favorite/data.json", 404, ""},
		{"GET", "/tool/poetrymatrix/", 404, ""},
		{"GET", "/tool/nextjlc/private.db", 404, ""},
		{"GET", "/tool/nextjlc/.secret.json", 404, ""},
		{"GET", "/tool/nextjlc/assets/main.js.map", 404, ""},
		{"GET", "/tool/nextjlc/../nextjlc/index.html", 404, ""},
		{"GET", "/tool/nextjlc/%252e%252e/index.html", 404, ""},
		{"GET", "/tool/nextjlc/assets%5cmain.js", 404, ""},
		{"POST", "/tool/nextjlc/", 405, ""},
	}
	for _, c := range cases {
		t.Run(c.method+c.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(c.method, c.path, nil)
			h.ServeHTTP(w, r)
			if w.Code != c.status {
				t.Fatalf("status %d, want %d; %s", w.Code, c.status, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), c.content) {
				t.Errorf("missing content %q", c.content)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Error("cache policy lost")
			}
			if strings.HasSuffix(c.path, "core.wasm") && w.Header().Get("Content-Type") != "application/wasm" {
				t.Error("incorrect WASM MIME")
			}
			if c.method == "HEAD" && w.Body.Len() != 0 {
				t.Error("HEAD returned body")
			}
		})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/tool?from=home", nil))
	if w.Header().Get("Location") != "/tool/?from=home" {
		t.Error("redirect dropped query")
	}
}

func TestCatalogVisibility(t *testing.T) {
	for _, tc := range []struct {
		name           string
		allowed        bool
		count, planned string
	}{
		{"visitor", false, "4", "3"},
		{"owner", true, "7", "5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := New(Options{CanViewPrivate: func(*http.Request) bool { return tc.allowed }})
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/tool/", nil))
			body := w.Body.String()
			if w.Code != 200 {
				t.Fatalf("status %d", w.Code)
			}
			if got := strconv.Itoa(strings.Count(body, "data-status=")); got != tc.count {
				t.Fatalf("cards %s, want %s", got, tc.count)
			}
			if strings.Contains(body, "浏览足迹") != tc.allowed || strings.Contains(body, "私人收藏") != tc.allowed {
				t.Error("private entries have wrong visibility")
			}
			if strings.Contains(body, `class="tools-filters"`) || strings.Contains(body, `data-filter=`) {
				t.Error("filter controls remain in tool catalog")
			}
			if strings.Contains(body, "公开工具") || strings.Contains(body, "私人空间") || strings.Contains(body, "工具中心") {
				t.Error("old section or title remains")
			}
			if !strings.Contains(body, "在线erber 文件，整理、转换并下载为jlc格式；无需上传电路文件。") {
				t.Error("description differs from requested text")
			}
			for _, path := range []string{"/tool/history/", "/tool/favorite/"} {
				r := httptest.NewRecorder()
				h.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
				if r.Code != 404 {
					t.Error("unimplemented private tool became accessible")
				}
			}
		})
	}
}

func TestSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nextjlc"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.json"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.json"), filepath.Join(root, "nextjlc", "escape.json")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	w := httptest.NewRecorder()
	New(Options{Root: root}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tool/nextjlc/escape.json", nil))
	if w.Code != 404 {
		t.Fatalf("symlink escape returned %d", w.Code)
	}
}
