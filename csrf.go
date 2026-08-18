// csrf.go — V12 (A3) CSRF 防护中间件
// 策略（对无状态 JSON API 最稳的组合）：
//   1. 所有非安全方法(POST/PATCH/DELETE/PUT)校验 Origin / Referer 与 Host 同源
//   2. 会话 Cookie 使用 SameSite=Strict（登录接口处设置）
// 说明：浏览器跨站请求会带 Origin；同源请求(或本地 curl/node 无来源头)不受影响。
package main

import (
	"net/http"
	"net/url"
)

// isSameOrigin 检查请求来源（Origin 或 Referer）与 Host 是否同源
func isSameOrigin(r *http.Request) bool {
	host := r.Host
	// 1) Origin（fetch/xhr 跨站必带）
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		if err != nil {
			return false
		}
		return u.Host == host
	}
	// 2) Referer（表单提交会带）
	if ref := r.Header.Get("Referer"); ref != "" {
		u, err := url.Parse(ref)
		if err != nil {
			return false
		}
		return u.Host == host
	}
	// 3) 无来源头（curl、node 等非浏览器）：放行，不误伤 API 测试
	return true
}

// csrfGuard 包装任意 handler：非安全方法先做同源校验
func csrfGuard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := r.Method
		if m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions {
			if !isSameOrigin(r) {
				apiErr(w, 403, "请求来源校验失败")
				return
			}
		}
		h(w, r)
	}
}