package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFRejectsCrossSiteByFetchMetadata(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	r.Host = "qdwasteland.top"
	// 没有任何 Origin/Referer，但浏览器明确说这是跨站请求
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	if isSameOrigin(r) {
		t.Fatal("cross-site 必须被拒绝（P2-7）")
	}
}

func TestCSRFAllowsSameOrigin(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	r.Host = "qdwasteland.top"
	r.Header.Set("Origin", "https://qdwasteland.top")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if !isSameOrigin(r) {
		t.Fatal("同源请求应放行")
	}
}

func TestCSRFRejectsForeignOrigin(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	r.Host = "qdwasteland.top"
	r.Header.Set("Origin", "https://evil.example")
	if isSameOrigin(r) {
		t.Fatal("跨站 Origin 必须被拒绝")
	}
}
func TestSameOriginGuardCoversToolRoutes(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	guard := sameOriginGuard(ok)

	cases := []struct {
		name   string
		method string
		path   string
		origin string
		want   int
	}{
		{"跨站 POST 工具接口", http.MethodPost, "/tool/history/api/devices", "https://evil.example", http.StatusForbidden},
		{"同源 POST 工具接口", http.MethodPost, "/tool/history/api/devices", "https://qdwasteland.top", http.StatusNoContent},
		{"GET 不校验", http.MethodGet, "/tool/history/api/devices", "https://evil.example", http.StatusNoContent},
		{"扩展同步豁免", http.MethodPost, "/api/history/sync", "https://evil.example", http.StatusNoContent},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		req.Host = "qdwasteland.top"
		req.Header.Set("Origin", c.origin)
		rec := httptest.NewRecorder()
		guard.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s：got %d, want %d", c.name, rec.Code, c.want)
		}
	}
}
