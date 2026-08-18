// topic.go — F4 专题管理：文章归入专题，专题页聚合
//
// 表：
//   topics(id,title,description,created_by,created_at)
//   topic_articles(topic_id,article_id)
//
// 接口：
//   GET    /api/topics                      公开专题列表
//   GET    /api/topics/{id}                 专题详情（聚合该专题下 status='正常' 的文章）
//   POST   /api/admin/topics                新建专题（仅管理员）
//   POST   /api/admin/topics/{id}/articles  专题中加入/移除文章 body:{articleId,action:add|remove}
//   DELETE /api/admin/topics/{id}           删除专题（仅管理员）
//   GET    /api/admin/topic-articles        后台可选文章列表（仅管理员，供下拉选择）
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func migrateTopicsTables(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS topics (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		title       TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		created_by  INTEGER NOT NULL,
		created_at  TEXT NOT NULL
	)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS topic_articles (
		topic_id   INTEGER NOT NULL,
		article_id INTEGER NOT NULL,
		PRIMARY KEY (topic_id, article_id)
	)`)
}

// ---- 公开：专题列表 ----
func handleListTopics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/api/topics/")
		if p != "" && p != r.URL.Path {
			if id := parseID(p); id != 0 {
				handleTopicDetail(w, r, id)
				return
			}
		}
		// ⚠️ SQLite 单连接铁律：先完整取出 rows 并 Close，再做第二次查询，
		// 否则在同一 handler 内第二次 db 调用会死锁。故先收集专题，关闭 rows，再查计数。
		page := atoi(r.URL.Query().Get("page"))
		if page < 1 { page = 1 }
		pageSize := atoi(r.URL.Query().Get("pageSize"))
		if pageSize < 1 { pageSize = 200 } // 不传参数时返回全部（兼容公开页面）
		if pageSize > 200 { pageSize = 200 }
		var total int
		authStore.db.QueryRow(`SELECT COUNT(*) FROM topics`).Scan(&total)
		rows, err := authStore.db.Query(`SELECT id,title,description,created_by,created_at FROM topics ORDER BY id DESC LIMIT ? OFFSET ?`, pageSize, (page-1)*pageSize)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		type item struct {
			ID          int64  `json:"id"`
			Title       string `json:"title"`
			Description string `json:"description"`
			CreatedAt   string `json:"createdAt"`
			ArticleCount int   `json:"articleCount"`
		}
		var items []item
		for rows.Next() {
			it := item{}
			var by int64
			if err := rows.Scan(&it.ID, &it.Title, &it.Description, &by, &it.CreatedAt); err == nil {
				items = append(items, it)
			}
		}
		rows.Close() // 必须先关闭，才能复用连接

		for i := range items {
			_ = authStore.db.QueryRow(`SELECT COUNT(*) FROM topic_articles WHERE topic_id=?`, items[i].ID).Scan(&items[i].ArticleCount)
		}
		apiJSON(w, 200, map[string]any{"total": total, "topics": items})
	}
}

// ---- 公开：专题详情（聚合文章）----
func handleTopicDetail(w http.ResponseWriter, r *http.Request, id int64) {
	var title, desc, created string
	var by int64
	err := authStore.db.QueryRow(`SELECT title,description,created_by,created_at FROM topics WHERE id=?`, id).
		Scan(&title, &desc, &by, &created)
	if err != nil {
		apiErr(w, 404, "专题不存在")
		return
	}
	rows, err := authStore.db.Query(`SELECT a.id,a.title,a.category,a.created_at,COALESCE(u.nickname,'')
		FROM topic_articles ta
		JOIN articles a ON a.id=ta.article_id
		LEFT JOIN users u ON u.id=a.user_id
		WHERE ta.topic_id=? AND a.status='正常' AND COALESCE(u.banned,0)=0
		ORDER BY a.id DESC`, id)
	if err != nil {
		apiErr(w, 500, "查询失败")
		return
	}
	defer rows.Close()
	type art struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		Category  string `json:"category"`
		Author    string `json:"author"`
		CreatedAt string `json:"createdAt"`
	}
	var articles []art
	for rows.Next() {
		a := art{}
		if err := rows.Scan(&a.ID, &a.Title, &a.Category, &a.CreatedAt, &a.Author); err == nil {
			if a.Author == "" {
				a.Author = "匿名"
			}
			articles = append(articles, a)
		}
	}
	apiJSON(w, 200, map[string]any{
		"topic":    map[string]any{"id": id, "title": title, "description": desc, "createdAt": created},
		"articles": articles,
	})
}

// ---- 后台：新建专题 ----
func handleAdminCreateTopic(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		var body struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		body.Title = strings.TrimSpace(body.Title)
		if body.Title == "" {
			apiErr(w, 400, "专题标题不能为空")
			return
		}
		res, err := auth.db.Exec(`INSERT INTO topics (title,description,created_by,created_at) VALUES (?,?,?,?)`,
			body.Title, strings.TrimSpace(body.Description), currentUserID(r), time.Now().Format("2006-01-02 15:04:05"))
		if err != nil {
			apiErr(w, 500, "创建失败")
			return
		}
		id, _ := res.LastInsertId()
		apiJSON(w, 200, map[string]any{"ok": true, "id": id})
	}
}

// ---- 后台：专题加入/移除文章 ----
func handleAdminTopicArticles(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/api/admin/topics/")
		if r.Method == http.MethodDelete {
			id := parseID(strings.TrimRight(p, "/"))
			if id == 0 {
				apiErr(w, 400, "无效专题ID")
				return
			}
			_, _ = auth.db.Exec(`DELETE FROM topic_articles WHERE topic_id=?`, id)
			_, _ = auth.db.Exec(`DELETE FROM topics WHERE id=?`, id)
			apiJSON(w, 200, map[string]any{"ok": true})
			return
		}
		idStr := strings.TrimSuffix(p, "/articles")
		id := parseID(idStr)
		if id == 0 {
			apiErr(w, 400, "无效专题ID")
			return
		}
		var body struct {
			ArticleID int64  `json:"articleId"`
			Action    string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		if body.ArticleID == 0 {
			apiErr(w, 400, "缺少 articleId")
			return
		}
		if body.Action == "add" {
			_, _ = auth.db.Exec(`INSERT OR IGNORE INTO topic_articles (topic_id,article_id) VALUES (?,?)`, id, body.ArticleID)
		} else if body.Action == "remove" {
			_, _ = auth.db.Exec(`DELETE FROM topic_articles WHERE topic_id=? AND article_id=?`, id, body.ArticleID)
		} else {
			apiErr(w, 400, "action 必须为 add 或 remove")
			return
		}
		apiJSON(w, 200, map[string]any{"ok": true})
	}
}

// ---- 后台：可选文章列表（供下拉选择，仅 status='正常'）----
func handleAdminTopicCandidateArticles(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		rows, err := auth.db.Query(`SELECT id,title FROM articles WHERE status='正常' ORDER BY id DESC LIMIT 200`)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		defer rows.Close()
		type a struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		}
		var out []a
		for rows.Next() {
			it := a{}
			if err := rows.Scan(&it.ID, &it.Title); err == nil {
				out = append(out, it)
			}
		}
		apiJSON(w, 200, map[string]any{"articles": out})
	}
}
