package placeholders

import (
	"database/sql"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"wasteland/internal/tools"
)

func init() {
	tools.Register(favoriteTool{})
	tools.Register(entry{meta: tools.Meta{Slug: "poetrymatrix", Title: "PoetryMatrix", Summary: "为诗意留一块地方。独立工具准备好后从这里接入。", Label: "POETRY", Access: tools.Public, Enabled: false}})
}

type favoriteTool struct{}

func (favoriteTool) Meta() tools.Meta {
	// 私人收藏尚未完成产品设计（分组、导入、隐私边界都还没定），
	// 先按"准备中"展示：清单页保留说明，但不挂路由，避免半成品上线。
	return tools.Meta{Slug: "favorite", Title: "私人收藏", Summary: "仅本人可见的网址收藏。功能仍在准备中，暂不开放。", Label: "PRIVATE / BOOKMARKS", Access: tools.Member, Enabled: false}
}
func (favoriteTool) Register(mux *http.ServeMux, env tools.Env) {
	if env.DB == nil {
		panic("favorite: shared database is unavailable")
	}
	if _, err := env.DB.Exec(`CREATE TABLE IF NOT EXISTS tool_bookmarks (id INTEGER PRIMARY KEY AUTOINCREMENT,user_id INTEGER NOT NULL,title TEXT NOT NULL,url TEXT NOT NULL,description TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,updated_at TEXT NOT NULL,UNIQUE(user_id,url))`); err != nil {
		panic("favorite: create schema: " + err.Error())
	}
	mux.HandleFunc("/api/bookmarks", func(w http.ResponseWriter, r *http.Request) { handleBookmarks(env.DB, env.UserID, w, r) })
	mux.HandleFunc("/api/bookmarks/", func(w http.ResponseWriter, r *http.Request) { handleBookmarkItem(env.DB, env.UserID, w, r) })
	mux.HandleFunc("/", renderFavoritePage)
}

type bookmark struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}
type bookmarkInput struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

func handleBookmarks(db *sql.DB, resolve func(*http.Request) int64, w http.ResponseWriter, r *http.Request) {
	uid := getUID(resolve, r)
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`SELECT id,title,url,description,created_at,updated_at FROM tool_bookmarks WHERE user_id=? ORDER BY updated_at DESC,id DESC`, uid)
		if err != nil {
			bookmarkError(w, 500, "读取收藏失败")
			return
		}
		defer rows.Close()
		items := make([]bookmark, 0)
		for rows.Next() {
			var item bookmark
			if err := rows.Scan(&item.ID, &item.Title, &item.URL, &item.Description, &item.CreatedAt, &item.UpdatedAt); err != nil {
				bookmarkError(w, 500, "读取收藏失败")
				return
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			bookmarkError(w, 500, "读取收藏失败")
			return
		}
		bookmarkJSON(w, 200, map[string]any{"items": items})
	case http.MethodPost:
		var input bookmarkInput
		if err := decodeBookmark(w, r, &input); err != nil {
			bookmarkError(w, 400, err.Error())
			return
		}
		if err := validateBookmark(&input); err != nil {
			bookmarkError(w, 400, err.Error())
			return
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		res, err := db.Exec(`INSERT INTO tool_bookmarks(user_id,title,url,description,created_at,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(user_id,url) DO UPDATE SET title=excluded.title,description=excluded.description,updated_at=excluded.updated_at`, uid, input.Title, input.URL, input.Description, now, now)
		if err != nil {
			bookmarkError(w, 500, "保存收藏失败")
			return
		}
		id, _ := res.LastInsertId()
		if id == 0 {
			_ = db.QueryRow(`SELECT id FROM tool_bookmarks WHERE user_id=? AND url=?`, uid, input.URL).Scan(&id)
		}
		bookmarkJSON(w, 201, map[string]any{"id": id})
	default:
		w.Header().Set("Allow", "GET, POST")
		bookmarkError(w, 405, "只支持 GET 或 POST")
	}
}
func handleBookmarkItem(db *sql.DB, resolve func(*http.Request) int64, w http.ResponseWriter, r *http.Request) {
	uid := getUID(resolve, r)
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/bookmarks/"), 10, 64)
	if err != nil || id <= 0 {
		bookmarkError(w, 400, "收藏编号无效")
		return
	}
	switch r.Method {
	case http.MethodPut, http.MethodPatch:
		var input bookmarkInput
		if err := decodeBookmark(w, r, &input); err != nil {
			bookmarkError(w, 400, err.Error())
			return
		}
		if err := validateBookmark(&input); err != nil {
			bookmarkError(w, 400, err.Error())
			return
		}
		res, err := db.Exec(`UPDATE tool_bookmarks SET title=?,url=?,description=?,updated_at=? WHERE id=? AND user_id=?`, input.Title, input.URL, input.Description, time.Now().UTC().Format(time.RFC3339Nano), id, uid)
		if err != nil {
			bookmarkError(w, 500, "更新收藏失败")
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			bookmarkError(w, 404, "收藏不存在")
			return
		}
		bookmarkJSON(w, 200, map[string]bool{"updated": true})
	case http.MethodDelete:
		res, err := db.Exec(`DELETE FROM tool_bookmarks WHERE id=? AND user_id=?`, id, uid)
		if err != nil {
			bookmarkError(w, 500, "删除收藏失败")
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			bookmarkError(w, 404, "收藏不存在")
			return
		}
		bookmarkJSON(w, 200, map[string]bool{"deleted": true})
	default:
		w.Header().Set("Allow", "PUT, PATCH, DELETE")
		bookmarkError(w, 405, "请求方法不支持")
	}
}
func getUID(resolve func(*http.Request) int64, r *http.Request) int64 {
	if resolve == nil {
		return 0
	}
	return resolve(r)
}
func decodeBookmark(w http.ResponseWriter, r *http.Request, out *bookmarkInput) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return errors.New("请求格式错误")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("请求格式错误")
	}
	return nil
}
func validateBookmark(b *bookmarkInput) error {
	b.Title = strings.TrimSpace(b.Title)
	b.URL = strings.TrimSpace(b.URL)
	b.Description = strings.TrimSpace(b.Description)
	if b.Title == "" || len(b.Title) > 200 {
		return errors.New("标题长度须为 1 到 200 字符")
	}
	if len(b.Description) > 1000 {
		return errors.New("描述不能超过 1000 字符")
	}
	parsed, err := url.ParseRequestURI(b.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || len(b.URL) > 2048 {
		return errors.New("网址必须是有效的 HTTP 或 HTTPS 地址")
	}
	return nil
}
func bookmarkJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func bookmarkError(w http.ResponseWriter, status int, message string) {
	bookmarkJSON(w, status, map[string]string{"error": message})
}

var favoriteTemplate = template.Must(template.New("favorite").Parse(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>私人收藏 · Wasteland</title><link rel="icon" href="/favicon.svg"><link rel="stylesheet" href="/style.css"><link rel="stylesheet" href="/nav.css"><link rel="stylesheet" href="/common.css"><link rel="stylesheet" href="/themes.css"><link rel="stylesheet" href="/tool/_assets/tool-center.css"><link rel="stylesheet" href="/tool/favorite/style.css"><script defer src="/icons.js"></script><script defer src="/nav.js"></script><script defer src="/tool/favorite/app.js"></script></head><body class="tools-body favorite-body"><div id="site-nav"></div><main class="tools-main favorite-main"><nav class="tools-breadcrumb"><a href="/tool/">返回工具</a></nav><header class="tools-heading"><p class="tools-kicker">PRIVATE / BOOKMARKS</p><h1>私人收藏</h1></header><section class="bookmark-editor"><form id="bookmark-form"><input type="hidden" name="id"><label>名称<input name="title" maxlength="200" required></label><label>网址<input name="url" type="url" maxlength="2048" placeholder="https://example.com" required></label><label>备注<input name="description" maxlength="1000"></label><button type="submit">保存</button><button type="button" id="bookmark-cancel" hidden>取消</button></form><p id="bookmark-message" role="status"></p></section><section><h2 class="bookmark-list-title">已收藏网址</h2><div id="bookmark-list" aria-live="polite"></div></section></main></body></html>`))

func renderFavoritePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		return
	}
	_ = favoriteTemplate.Execute(w, nil)
}

type entry struct{ meta tools.Meta }

func (e entry) Meta() tools.Meta                   { return e.meta }
func (e entry) Register(*http.ServeMux, tools.Env) {}
