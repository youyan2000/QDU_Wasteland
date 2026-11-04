// security_headers.go — 应用层安全响应头与请求体上限（审计 P2-4 / P2-6）。
//
// 之前这些完全依赖 nginx，但文档里的 nginx 配置一条响应头都没有，等于全站裸奔。
// 现在改为应用自己发头：换任何反代、甚至直接跑裸 HTTP 都有防护。
package main

import (
	"net/http"
	"os"
	"strings"
)

// 允许外部图片（B站封面等），其余一律同源。
const contentSecurityPolicy = "default-src 'self'; " +
	"img-src 'self' data: https:; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

// jsonBodyLimit 是 JSON 接口的请求体上限（上传走 multipart，另有自己的限制）。
const jsonBodyLimit = 2 << 20

// jsonBodyLimit 之外的请求（multipart 上传）给一个宽松上限，防止无限流式上传打满磁盘。
const rawBodyLimit = 256 << 20

// importBodyLimit 是"导入备份"这个接口的专属上限：浏览器导出的历史 JSON
// 动辄几 MB 到十几 MB，用 2MB 的通用 JSON 上限会直接掐断上传。
const importBodyLimit = 24 << 20

// securityHeaders 给所有响应加安全头，并给 JSON 请求套上请求体上限。
func securityHeaders(next http.Handler) http.Handler {
	// 未完成全部子域 HTTPS 盘点前，只约束当前主机，不申请 preload。
	hsts := "max-age=31536000"
	if os.Getenv("HSTS_INCLUDE_SUBDOMAINS") == "true" {
		hsts += "; includeSubDomains"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		// 只在 HTTPS 下有意义；本地 http 调试时浏览器会忽略。
		h.Set("Strict-Transport-Security", hsts)
		// 页面内联了少量 style，脚本全部是外部文件，所以脚本不需要 unsafe-inline。
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/me" {
			h.Set("Content-Security-Policy", contentSecurityPolicy)
		}

		if r.Body != nil {
			limit := int64(rawBodyLimit)
			if ct := r.Header.Get("Content-Type"); strings.Contains(ct, "application/json") {
				limit = jsonBodyLimit
				if strings.HasSuffix(r.URL.Path, "/import/preview") {
					limit = importBodyLimit
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}
