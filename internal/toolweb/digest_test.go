package toolweb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"qdu-wasteland/internal/githubdigest"
)

func writeDigestFixture(t *testing.T) *githubdigest.Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "digest"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := `{"date":"2026-10-21","generatedAt":"2026-10-21T08:00:00+08:00","items":[` +
		`{"name":"board","fullName":"acme/board","url":"https://github.com/acme/board",` +
		`"description":"KiCad PCB templates","stars":1200,"language":"C++",` +
		`"topics":["kicad","pcb"],"pushedAt":"2026-10-21T01:00:00Z","avatar":""}]}`
	for _, name := range []string{"latest.json", "2026-10-21.json"} {
		if err := os.WriteFile(filepath.Join(dir, "digest", name), []byte(payload), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return githubdigest.NewStore(dir, "")
}

func TestDigestRouteOwnerOnly(t *testing.T) {
	store := writeDigestFixture(t)

	owner := New(Options{Digest: store, CanViewPrivate: func(*http.Request) bool { return true }})
	w := httptest.NewRecorder()
	owner.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tool/digest/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("owner status %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "acme/board") {
		t.Fatal("digest repository missing from page")
	}
	if !strings.Contains(w.Body.String(), "开源日报") {
		t.Fatal("digest page title missing")
	}

	visitor := New(Options{Digest: store, CanViewPrivate: func(*http.Request) bool { return false }})
	w2 := httptest.NewRecorder()
	visitor.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/tool/digest/", nil))
	if w2.Code != http.StatusNotFound {
		t.Fatalf("visitor status %d, want 404", w2.Code)
	}
	if strings.Contains(w2.Body.String(), "acme/board") {
		t.Fatal("private digest leaked to visitor")
	}
}

func TestDigestRouteWithoutStore(t *testing.T) {
	h := New(Options{CanViewPrivate: func(*http.Request) bool { return true }})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tool/digest/", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404 when digest disabled", w.Code)
	}
}

func TestDigestRoutePicksRequestedDay(t *testing.T) {
	store := writeDigestFixture(t)
	h := New(Options{Digest: store, CanViewPrivate: func(*http.Request) bool { return true }})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tool/digest/?date=2026-10-21", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "2026-10-21") {
		t.Fatal("requested digest date missing")
	}
}
