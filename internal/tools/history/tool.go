// tool.go — history 工具的路由装配。
//
// 页面是上游 unlimited_history 的本地网页（原生 JS，无构建），
// 接口挂在 /tool/history/api/ 之下，只有站长能进。
package history

import (
	"embed"
	"io/fs"
	"net/http"

	"wasteland/internal/tools"
)

//go:embed static
var assets embed.FS

// Server 承载全部 HTTP 处理器。
type Server struct{ store *Store }

// NewServer 绑定存储。
func NewServer(st *Store) *Server { return &Server{store: st} }

var server *Server

// Setup 由主程序在挂载前调用：建表并绑定数据库。
func Setup(st *Store) { server = NewServer(st) }

func init() { tools.Register(tool{}) }

type tool struct{}

func (tool) Meta() tools.Meta {
	return tools.Meta{
		Slug:    "history",
		Title:   "浏览足迹",
		Summary: "浏览器历史、B站历史与书签，仅本人可见。",
		Label:   "PRIVATE / HISTORY",
		Access:  tools.OwnerOnly,
		Enabled: true,
	}
}

// pages 是 SPA 的四个入口，全部落到同一份 index.html。
var pages = map[string]bool{
	"/": true, "/index.html": true,
	"/browser/": true, "/bilibili/": true, "/favorite/": true, "/backup/": true,
}

func (tool) Register(mux *http.ServeMux, _ tools.Env) {
	s := server
	if s == nil {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
		return
	}

	mux.HandleFunc("/api/me", s.handleMe)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/history", s.handleHistory)
	mux.HandleFunc("/api/favorite", s.handleFavorites)
	mux.HandleFunc("/api/favorite/folders", s.handleFavoriteFolders)
	mux.HandleFunc("/api/favorite/snapshots", s.handleFavoriteSnapshots)
	mux.HandleFunc("/api/import/preview", s.handleImportPreview)
	mux.HandleFunc("/api/import/confirm", s.handleImportConfirm)
	mux.HandleFunc("/api/backup", s.handleBackup)

	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic("history: 静态资源缺失: " + err.Error())
	}
	fileServer := tools.Static{FS: sub, CacheControl: "no-store"}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		panic("history: index.html 缺失: " + err.Error())
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if pages[r.URL.Path] {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if r.Method == http.MethodHead {
				return
			}
			_, _ = w.Write(index)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
