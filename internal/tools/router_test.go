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
	// 未启用工具不显示「可使用 / 待接入」角标，也不在卡片底部放「准备中」文字，
	// 只用禁用态 + title 提示。
	if strings.Contains(body, "tool-status") || strings.Contains(body, "tool-pending") {
		t.Error("未启用工具不应再渲染状态角标/准备中文案")
	}
	if !strings.Contains(body, `title="准备中"`) {
		t.Error("未启用工具应保留 title 提示，便于鼠标悬停时说明")
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

func TestMemberRedirectsAnonymousPageAndReturns401ForAPI(t *testing.T) {
	d := newTestDispatcher(t, Env{}, Meta{Slug: "private", Title: "Private", Access: Member, Enabled: true})

	page := httptest.NewRecorder()
	d.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/tool/private/?tab=recent", nil))
	if page.Code != http.StatusFound {
		t.Fatalf("匿名页面请求应跳登录，got %d", page.Code)
	}
	if got := page.Header().Get("Location"); got != "/login.html?next=%2Ftool%2Fprivate%2F%3Ftab%3Drecent" {
		t.Fatalf("登录回跳地址不正确：%q", got)
	}

	api := httptest.NewRecorder()
	d.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/tool/private/api/ping", nil))
	if api.Code != http.StatusUnauthorized || !strings.Contains(api.Body.String(), "请先登录") {
		t.Fatalf("匿名 API 应返回 401 JSON，got %d %q", api.Code, api.Body.String())
	}
}

func TestMemberAllowedForAuthenticatedUserAndListedInPortal(t *testing.T) {
	env := Env{UserID: func(*http.Request) int64 { return 42 }}
	d := newTestDispatcher(t, env, Meta{Slug: "private", Title: "Private", Access: Member, Enabled: true})

	page := httptest.NewRecorder()
	d.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/tool/private/", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("登录用户应可访问，got %d", page.Code)
	}

	portal := httptest.NewRecorder()
	d.ServeHTTP(portal, httptest.NewRequest(http.MethodGet, "/tool/", nil))
	if !strings.Contains(portal.Body.String(), "Private") {
		t.Fatal("登录用户应在清单页看到 Member 工具")
	}
}

func TestMemberHiddenFromAnonymousPortal(t *testing.T) {
	d := newTestDispatcher(t, Env{}, Meta{Slug: "private", Title: "Private", Access: Member, Enabled: true})
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tool/", nil))
	if strings.Contains(rec.Body.String(), "Private") {
		t.Fatal("匿名用户不应在清单页看到 Member 工具")
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
