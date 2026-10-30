package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// GET 不应触发状态变更（审计 P2-8）：这三个接口都必须拒绝非 POST。
func TestStateChangingEndpointsRejectGET(t *testing.T) {
	if authStore == nil {
		// 这些 handler 依赖 authStore 读会话；未初始化时只需验证方法校验在最前面。
		t.Skip("authStore 未初始化")
	}
	cases := []struct {
		name string
		h    http.HandlerFunc
		path string
	}{
		{"forum/like", handleLike(), "/api/forum/like?id=1"},
		{"messages/read", handleReadMessages(), "/api/messages/read"},
		{"logout", handleLogout(), "/api/logout"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		c.h(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: GET 应返回 405，实际 %d", c.name, rec.Code)
		}
	}
}