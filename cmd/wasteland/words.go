package main

import (
	"net/http"
	"strconv"

	"wasteland/internal/wordbank"
)

// handleRandomWords 返回随机词条，供全站"每日单词"条使用。
// 词库编译进二进制，不读取本地目录，也不依赖外部网络。
func handleRandomWords() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			apiErr(w, http.StatusMethodNotAllowed, "只支持 GET")
			return
		}
		n := 2
		if v := r.URL.Query().Get("n"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 5 {
				n = parsed
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		apiJSON(w, http.StatusOK, map[string]any{"words": wordbank.Pick(n)})
	}
}
