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