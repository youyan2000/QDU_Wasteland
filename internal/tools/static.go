package tools

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// Static 把工具自己的前端产物挂到一个路径上。
//
// 用法（在工具的 Register 里）：
//
//	//go:embed static/*
//	var assets embed.FS
//	sub, _ := fs.Sub(assets, "static")
//	mux.Handle("/", tools.Static{FS: sub})
//
// 行为：
//   - 目录路径自动补 index.html；
//   - 只允许白名单扩展名，杜绝把 .go/.env 之类误发出去；
//   - 拒绝 .. 与反斜杠，不允许跳出 embed 根；
//   - 默认 no-store（本站工具多为私有数据），可显式放开缓存。
type Static struct {
	FS           fs.FS
	CacheControl string
}

// allowedExt 是允许对外提供的静态文件类型。
var allowedExt = map[string]bool{
	".html": true, ".htm": true, ".js": true, ".mjs": true, ".css": true,
	".json": true, ".svg": true, ".png": true, ".jpg": true, ".jpeg": true,
	".webp": true, ".gif": true, ".ico": true, ".woff": true, ".woff2": true,
	".ttf": true, ".wasm": true, ".map": true, ".txt": true, ".webmanifest": true,
}

func (s Static) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.FS == nil {
		http.NotFound(w, r)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if strings.Contains(name, "\\") {
		http.NotFound(w, r)
		return
	}
	name = path.Clean(name)
	if name == "." || name == ".." || strings.HasPrefix(name, "../") || !fs.ValidPath(name) {
		http.NotFound(w, r)
		return
	}

	// 目录请求补 index.html。
	if info, err := fs.Stat(s.FS, name); err == nil && info.IsDir() {
		name = path.Join(name, "index.html")
	}

	ext := strings.ToLower(path.Ext(name))
	if !allowedExt[ext] {
		http.NotFound(w, r)
		return
	}

	data, err := fs.ReadFile(s.FS, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if ct := mime.TypeByExtension(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	cache := s.CacheControl
	if cache == "" {
		cache = "no-store"
	}
	w.Header().Set("Cache-Control", cache)

	// 零 modtime：避免把构建时间泄露成 Last-Modified。
	http.ServeContent(w, r, path.Base(name), time.Time{}, strings.NewReader(string(data)))
}
