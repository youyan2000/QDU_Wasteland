// assets.go — 工具中心共用的静态资源与页面模板。
//
// 所有工具的 HTML/CSS/JS/封面图都打在这里，一个 embed 供全部工具使用：
// 工具包通过 AssetsFS()/ParsePage() 取自己需要的那几份，避免每个包各自维护一份 embed。
package tools

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed static
var assets embed.FS

// AssetsFS 返回 static/ 子树，供工具读取自己的页面与数据文件。
func AssetsFS() fs.FS {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic("tools: 静态资源子树缺失: " + err.Error())
	}
	return sub
}

// ParsePage 解析 static/ 下的模板文件，并注入工具页共用的模板函数。
//
// 注意：template.ParseFS 会按文件名建立子模板，根模板本身是空的。
// 只传一个文件时直接返回该子模板，否则 Execute 会输出空内容（页面显示空白）。
func ParsePage(names ...string) *template.Template {
	t := template.New("page").Funcs(template.FuncMap{"when": whenLabel})
	for _, name := range names {
		t = template.Must(t.ParseFS(assets, "static/"+name))
	}
	if len(names) == 1 {
		if sub := t.Lookup(path.Base(names[0])); sub != nil {
			return sub
		}
	}
	return t
}

// whenLabel 把 ISO 时间渲染成"几分钟前"这类相对时间。
func whenLabel(iso string) string {
	if iso == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < 0:
		return t.Format("01-02 15:04")
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return itoa(int(d.Minutes())) + " 分钟前"
	case d < 24*time.Hour:
		return itoa(int(d.Hours())) + " 小时前"
	case d < 48*time.Hour:
		return "昨天 " + t.Format("15:04")
	default:
		return t.Format("01-02")
	}
}

// itoa 避免为一次格式化引入 strconv 依赖（模板里只有小整数）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// assetFiles 是允许通过 /tool/_assets/ 对外提供的共享资源。
var assetFiles = map[string]bool{
	"tool-center.css":   true,
	"tool-center.js":    true,
	"site-shell.css":    true,
	"collection.css":    true,
	"collection.js":     true,
	"digest.css":        true,
	"digest.js":         true,
	"hot.css":           true,
	"hot.js":            true,
	"curated-bookmarks.json": false, // 数据文件，不直接对外
}

// serveAsset 处理 /tool/_assets/... 的共享资源请求。
func serveAsset(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/tool/_assets/")
	if p == "" || strings.ContainsAny(p, "\\%") || strings.HasPrefix(p, ".") {
		http.NotFound(w, r)
		return
	}
	// 封面图：/tool/_assets/covers/photos/xxx.jpg 与 covers/photo-sources/xxx.jpg
	if rest, ok := strings.CutPrefix(p, "covers/"); ok {
		if strings.HasPrefix(rest, "photos/") || strings.HasPrefix(rest, "photo-sources/") {
			if !strings.HasSuffix(rest, ".jpg") || strings.Count(rest, "/") != 1 {
				http.NotFound(w, r)
				return
			}
			Static{FS: AssetsFS(), CacheControl: "public, max-age=604800"}.ServeHTTP(w,
				withPath(r, "/"+p))
			return
		}
		http.NotFound(w, r)
		return
	}
	if !assetFiles[p] {
		http.NotFound(w, r)
		return
	}
	Static{FS: AssetsFS(), CacheControl: "public, max-age=60"}.ServeHTTP(w, withPath(r, "/"+p))
}

// withPath 克隆请求并替换路径，便于复用 Static。
func withPath(r *http.Request, p string) *http.Request {
	clone := r.Clone(r.Context())
	clone.URL.Path = p
	return clone
}
