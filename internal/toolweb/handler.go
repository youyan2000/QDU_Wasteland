// Package toolweb provides the public tools portal and explicitly registered bundles.
package toolweb

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"qdu-wasteland/internal/githubdigest"
)

//go:embed static/*
var assets embed.FS

var templates = template.Must(template.ParseFS(assets, "static/index.html", "static/collection.html", "static/digest.html"))
var portal = templates.Lookup("index.html")
var collectionPage = templates.Lookup("collection.html")
var digestPage = templates.Lookup("digest.html")

// Bookmark is a curated external reading/play site.
type Bookmark struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Cover       string `json:"cover"`
	Image       string `json:"image"`
}

type BookmarkCollections struct {
	Magazine []Bookmark `json:"magazine"`
	Toy      []Bookmark `json:"toy"`
}

var bookmarks = func() BookmarkCollections {
	data, err := assets.ReadFile("static/curated-bookmarks.json")
	if err != nil {
		panic(err)
	}
	var result BookmarkCollections
	if err := json.Unmarshal(data, &result); err != nil {
		panic(err)
	}
	return result
}()

type Tool struct {
	Slug, Title, Summary, Category, Label string
	Enabled                               bool
}

// Only shipped public tools are enabled. Private tools have no file-serving route.
var tools = []Tool{
	{"nextjlc", "NeXTJLC", "在线erber 文件，整理、转换并下载为jlc格式；无需上传电路文件。", "public", "PCB / GERBER", true},
	{"poetrymatrix", "PoetryMatrix", "为诗意留一块地方。独立工具准备好后，将从这里接入。", "public", "POETRY", false},
	{"information", "随笔散文", "杂志、文学、哲学、科学与技术：留一些值得慢慢读的地方。", "public", "READ / THINK", true},
	{"toy", "趣味角落", "一些好玩、古怪、可以点开探索的小网站。", "public", "JUST FOR FUN", true},
	{"digest", "开源日报", "每日 GitHub 热门开源项目，仅本人可见。", "private", "GITHUB / DAILY", true},
	{"history", "浏览足迹", "未来接入个人 B 站与浏览器历史，仅本人可见。当前未采集任何记录。", "private", "PRIVATE / HISTORY", false},
	{"favorite", "私人收藏", "未来接入仅本人可见的网址收藏，与公开导航分开存放。", "private", "PRIVATE / BOOKMARKS", false},
}

type digestView struct {
	Date, GeneratedAt, Today string
	Items                    []githubdigest.Item
	Days                     []string
	Empty                    bool
}

type Options struct {
	Root string
	Demo bool
	// CanViewPrivate resolves the existing site's session and owner permission.
	// Nil hides private entries; entries still have no route until implemented.
	CanViewPrivate func(*http.Request) bool
	// Digest serves the owner-only hardware digest page. Nil disables it.
	Digest *githubdigest.Store
}

// New returns a handler mountable at both /tool and /tool/.
// Root contains a directory per enabled slug, e.g. Root/nextjlc/index.html.
func New(opts Options) http.Handler {
	staticFS, _ := fs.Sub(assets, "static")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := r.URL.Path
		if p == "/tool" {
			redirect(w, r, "/tool/")
			return
		}
		if p == "/tool/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if r.Method == http.MethodHead {
				return
			}
			visible := make([]Tool, 0, len(tools))
			canViewPrivate := opts.CanViewPrivate != nil && opts.CanViewPrivate(r)
			available, planned := 0, 0
			for _, tool := range tools {
				if tool.Category != "public" && !canViewPrivate {
					continue
				}
				visible = append(visible, tool)
				if tool.Enabled {
					available++
				} else {
					planned++
				}
			}
			_ = portal.Execute(w, struct {
				Tools              []Tool
				Demo               bool
				Available, Planned int
			}{visible, opts.Demo, available, planned})
			return
		}
		if p == "/tool/information" {
			redirect(w, r, "/tool/information/")
			return
		}
		if p == "/tool/toy" {
			redirect(w, r, "/tool/toy/")
			return
		}
		if p == "/tool/information/" || p == "/tool/toy/" {
			if r.Method == http.MethodHead {
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=60")
			collection := "information"
			title := "随笔散文"
			intro := "杂志、文学、哲学、科学与技术：留一些值得慢慢读的地方。"
			items := bookmarks.Magazine
			if p == "/tool/toy/" {
				collection = "toy"
				title = "趣味角落"
				intro = "一些好玩、古怪、可以点开探索的小网站。"
				items = bookmarks.Toy
			}
			_ = collectionPage.Execute(w, struct {
				Title, Intro, Slug string
				Items              []Bookmark
			}{title, intro, collection, items})
			return
		}
		if p == "/tool/digest" {
			redirect(w, r, "/tool/digest/")
			return
		}
		if p == "/tool/digest/" {
			// Personal digest: never expose it to anonymous visitors or
			// ordinary logged-in users. 404 avoids confirming it exists.
			if opts.Digest == nil || opts.CanViewPrivate == nil || !opts.CanViewPrivate(r) {
				http.NotFound(w, r)
				return
			}
			if r.Method == http.MethodHead {
				return
			}
			view := digestView{Today: time.Now().Format("2006-01-02")}
			var d *githubdigest.Digest
			if day := r.URL.Query().Get("date"); day != "" {
				if picked, err := opts.Digest.Get(day); err == nil {
					d = picked
				}
			}
			if d == nil {
				if latest, err := opts.Digest.Latest(); err == nil {
					d = latest
				}
			}
			if d == nil || len(d.Items) == 0 {
				view.Empty = true
			} else {
				view.Date, view.GeneratedAt, view.Items = d.Date, d.GeneratedAt, d.Items
			}
			if days, err := opts.Digest.Days(); err == nil {
				if len(days) > 10 {
					days = days[:10]
				}
				view.Days = days
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "private, no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("X-Robots-Tag", "noindex, nofollow")
			_ = digestPage.Execute(w, view)
			return
		}
		if p == "/tool/_assets/tool-center.css" || p == "/tool/_assets/tool-center.js" || p == "/tool/_assets/site-shell.css" || p == "/tool/_assets/collection.css" || p == "/tool/_assets/collection.js" || p == "/tool/_assets/digest.css" || p == "/tool/_assets/digest.js" || ((strings.HasPrefix(p, "/tool/_assets/covers/") && strings.HasSuffix(p, ".svg") && !strings.Contains(strings.TrimPrefix(p, "/tool/_assets/covers/"), "/")) || (strings.HasPrefix(p, "/tool/_assets/covers/sites/") && strings.HasSuffix(p, ".svg") && !strings.Contains(strings.TrimPrefix(p, "/tool/_assets/covers/sites/"), "/")) || (strings.HasPrefix(p, "/tool/_assets/covers/photos/") && strings.HasSuffix(p, ".jpg") && !strings.Contains(strings.TrimPrefix(p, "/tool/_assets/covers/photos/"), "/"))) {
			rr := r.Clone(r.Context())
			rr.URL.Path = strings.TrimPrefix(p, "/tool/_assets")
			http.FileServer(http.FS(staticFS)).ServeHTTP(w, rr)
			return
		}
		if !strings.HasPrefix(p, "/tool/") {
			http.NotFound(w, r)
			return
		}
		rel := strings.TrimPrefix(p, "/tool/")
		if strings.ContainsAny(rel, "\\%") {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(rel, "/")
		var enabled bool
		for _, t := range tools {
			if t.Slug == parts[0] && t.Enabled && t.Category == "public" {
				enabled = true
				break
			}
		}
		if !enabled {
			http.NotFound(w, r)
			return
		}
		if len(parts) == 1 {
			redirect(w, r, p+"/")
			return
		}
		for i, part := range parts {
			if strings.HasPrefix(part, ".") || (part == "" && i != len(parts)-1) {
				http.NotFound(w, r)
				return
			}
		}
		name := strings.Join(parts[1:], "/")
		if name == "" {
			name = "index.html"
		}
		ext := strings.ToLower(filepath.Ext(name))
		allowed := map[string]bool{".html": true, ".js": true, ".css": true, ".wasm": true, ".ico": true, ".png": true, ".jpg": true, ".jpeg": true, ".svg": true, ".webp": true, ".woff": true, ".woff2": true, ".webmanifest": true, ".json": true}
		if !allowed[ext] || opts.Root == "" {
			http.NotFound(w, r)
			return
		}
		// Resolve symlinks and reject any file escaping this tool's registered root.
		root, err := filepath.EvalSymlinks(filepath.Join(opts.Root, parts[0]))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		path, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		within, err := filepath.Rel(root, path)
		if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) || filepath.IsAbs(within) {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || !stat.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		if ext == ".wasm" {
			w.Header().Set("Content-Type", "application/wasm")
		} else if ct := mime.TypeByExtension(ext); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
	})
}

func redirect(w http.ResponseWriter, r *http.Request, target string) {
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusPermanentRedirect)
}
