package tools

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestDispatcher 注册若干假工具并返回分发器。
func newTestDispatcher(t *testing.T, env Env, metas ...Meta) *dispatcher {
	t.Helper()
	resetForTest()
	t.Cleanup(resetForTest)

	for _, m := range metas {
		meta := m
		Register(&fakeTool{
			meta: meta,
			register: func(mux *http.ServeMux, _ Env) {
				mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.WriteString(w, "pong:"+meta.Slug)
				})
				mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.WriteString(w, "root:"+meta.Slug)
				})
			},
		})
	}
	return newDispatcher(env)
}

func owner(uid int64, allowed bool) Env {
	return Env{OwnerUID: uid, CanOwner: func(*http.Request) bool { return allowed }}
}

func TestToolRootRedirectsToSlash(t *testing.T) {
	d := newTestDispatcher(t, Env{})
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tool", nil))

	if rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("want 308, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/tool/" {
		t.Fatalf("want /tool/, got %q", loc)
	}
}

func TestSlugRedirectsToTrailingSlash(t *testing.T) {
	d := newTestDispatcher(t, Env{}, Meta{Slug: "demo", Title: "Demo", Enabled: true})
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tool/demo", nil))

	if rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("want 308, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/tool/demo/" {
		t.Fatalf("want /tool/demo/, got %q", loc)
	}
}

func TestPortalListsEnabledAndPlanned(t *testing.T) {
	d := newTestDispatcher(t, Env{},
		Meta{Slug: "alpha", Title: "Alpha", Summary: "s1", Label: "L1", Enabled: true},
		Meta{Slug: "beta", Title: "Beta", Summary: "s2", Label: "L2", Enabled: false},
	)

	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tool/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="/tool/alpha/"`) {
		t.Error("启用工具应有链接")
	}
	if strings.Contains(body, `href="/tool/beta/"`) {
		t.Error("未启用工具不应有链接")
	}
	// 模板用「待接入」角标 +「准备中」文案表示未启用。
	if !strings.Contains(body, "待接入") || !strings.Contains(body, "准备中") {
		t.Error("未启用工具应显示「待接入 / 准备中」")
	}
}

func TestPortalRejectsNonGet(t *testing.T) {
	d := newTestDispatcher(t, Env{})
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/tool/", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, "GET") {
		t.Fatalf("want Allow 头, got %q", allow)
	}
}

func TestUnknownSlugIs404(t *testing.T) {
	d := newTestDispatcher(t, Env{}, Meta{Slug: "demo", Title: "Demo", Enabled: true})
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tool/nope/", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestDisabledToolIs404(t *testing.T) {
	d := newTestDispatcher(t, Env{}, Meta{Slug: "later", Title: "Later", Enabled: false})
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tool/later/", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestOwnerOnlyDeniedWithoutOwner(t *testing.T) {
	d := newTestDispatcher(t, Env{}, Meta{Slug: "secret", Title: "Secret", Access: OwnerOnly, Enabled: true})
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tool/secret/", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("非站长应得到 404（不泄露存在性），got %d", rec.Code)
	}
}

func TestOwnerOnlyAllowedForOwner(t *testing.T) {
	d := newTestDispatcher(t, owner(7, true), Meta{Slug: "secret", Title: "Secret", Access: OwnerOnly, Enabled: true})
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tool/secret/api/ping", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("站长应可访问，got %d", rec.Code)
	}
	if got := rec.Body.String(); got != "pong:secret" {
		t.Fatalf("want pong:secret, got %q", got)
	}
}

func TestOwnerOnlyHiddenFromPortal(t *testing.T) {
	d := newTestDispatcher(t, Env{}, Meta{Slug: "secret", Title: "Secret", Access: OwnerOnly, Enabled: true})
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tool/", nil))

	if strings.Contains(rec.Body.String(), "Secret") {
		t.Fatal("非站长不应在清单页看到私有工具")
	}
}

func TestSlugRootServesWithoutRedirect(t *testing.T) {
	// 回归：/tool/x/ 曾经被判成「缺尾斜杠」，结果自己重定向给自己。
	d := newTestDispatcher(t, Env{}, Meta{Slug: "demo", Title: "Demo", Enabled: true})
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tool/demo/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("带斜杠的工具根路径应直接交给工具，got %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if got := rec.Body.String(); got != "root:demo" {
		t.Fatalf("want root:demo, got %q", got)
	}
}

func TestStripPrefixForTool(t *testing.T) {
	d := newTestDispatcher(t, Env{}, Meta{Slug: "demo", Title: "Demo", Enabled: true})
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tool/demo/api/ping", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if rec.Body.String() != "pong:demo" {
		t.Fatalf("工具应看到去掉 slug 的路径，got %q", rec.Body.String())
	}
}

func TestTraversalRejected(t *testing.T) {
	d := newTestDispatcher(t, Env{}, Meta{Slug: "demo", Title: "Demo", Enabled: true})

	for _, path := range []string{
		"/tool/demo/../x",
		"/tool/../tool/",
		"/tool/demo/%2e%2e/x",
		"/tool/demo\\x",
		"/tool/demo/.hidden",
	} {
		rec := httptest.NewRecorder()
		d.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("path %q 应被拒绝，got %d", path, rec.Code)
		}
	}
}

func TestOutsideToolPrefixIs404(t *testing.T) {
	d := newTestDispatcher(t, Env{})
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tooldemo/", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}
