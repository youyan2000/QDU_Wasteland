// static.go — public/ 静态前端与品牌 404 页。
//
// 抽成 newStaticHandler(dir) 是为了两件事：
//  1. 目录由 paths.go 的 publicDir() 显式传入，不再依赖进程工作目录；
//  2. 可被 httptest 直接覆盖，不必真起服务器（本机杀软会吃掉新编译的 exe）。
//
// 顺带修掉一个既有 bug：缺页时用 http.ServeFile 输出 404.html 会带着 200 状态码，
// 现在明确 w.WriteHeader(404)。
package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// newStaticHandler 返回服务 dir 下静态文件的处理器：
//
//	"/"、目录          → 该目录的 index.html
//	存在的文件          → 原样返回（交给 http.FileServer 处理 Range/缓存头）
//	其余                → 404 状态码 + dir/404.html
//
// dir 里连 404.html 都没有时退回 Go 自带的纯文本 404，便于判断是"目录没找对"。
func newStaticHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	notFoundPage := filepath.Join(dir, "404.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 禁止路径遍历
		if strings.Contains(r.URL.Path, "..") {
			http.NotFound(w, r)
			return
		}

		// Clean("/"+path) 保证在 Windows 上也是 /a/b 形式；dir 已是绝对路径，拼出来就是绝对路径
		target := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		fi, err := os.Stat(target)

		if err == nil && fi.IsDir() {
			index := filepath.Join(target, "index.html")
			if fi2, err2 := os.Stat(index); err2 == nil && !fi2.IsDir() {
				http.ServeFile(w, r, index)
				return
			}
			serveNotFound(w, r, notFoundPage)
			return
		}
		if err == nil {
			// 前端没有构建步骤，改完 CSS/JS 必须让浏览器重新校验，
			// 否则用户会长时间看到旧样式（"换了颜色没反应"大多是这个原因）。
			if ext := strings.ToLower(filepath.Ext(target)); ext == ".css" || ext == ".js" || ext == ".html" || ext == ".json" {
				w.Header().Set("Cache-Control", "no-cache")
			}
			files.ServeHTTP(w, r)
			return
		}
		serveNotFound(w, r, notFoundPage)
	})
}

// serveNotFound 输出品牌 404 页并保持 404 状态码。
func serveNotFound(w http.ResponseWriter, r *http.Request, page string) {
	body, err := os.ReadFile(page)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write(body)
}
