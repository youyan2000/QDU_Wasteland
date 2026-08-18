// article.go — V9 学校信息文章模块
// 分类：竞赛 / 生活 / 社团 / 其他（也允许自定义）
// 接口：
//   GET  /api/articles?cat=&q=          文章列表（只含 status='正常'）
//   GET  /api/articles/{id}             文章详情（含作者名）
//   POST /api/articles                  发布文章（登录，敏感词拦截）
//   GET  /api/admin/articles            后台文章列表（含下架）
//   POST /api/admin/articles/{id}/status 下架/恢复
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type articleDTO struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Category  string `json:"category"`
	Content   string `json:"content"`
	Author    string `json:"author"`
	Anonymous bool   `json:"anonymous"`
	Status      string `json:"status"`
	UserID      int64  `json:"userId"`
	RejectReason string `json:"rejectReason,omitempty"`
	Cover     string `json:"cover,omitempty"` // J7 封面URL
	CreatedAt   string `json:"createdAt"`
}

func handleListArticles() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 详情路由：/api/articles/{id}
		p := strings.TrimPrefix(r.URL.Path, "/api/articles/")
		if p != "" && p != r.URL.Path {
			if id := parseID(p); id != 0 {
				getArticleDetail(w, r, id)
				return
			}
		}
		cat := r.URL.Query().Get("cat")
		q := r.URL.Query().Get("q")
		sqlq := `SELECT a.id, a.title, a.category, a.content, a.is_anonymous, a.status, a.user_id, a.created_at, a.cover,
			COALESCE(u.nickname,'') FROM articles a LEFT JOIN users u ON u.id=a.user_id
			WHERE a.status='正常' AND COALESCE(u.banned,0)=0`
		args := []any{}
		if cat != "" {
			sqlq += " AND a.category=?"
			args = append(args, cat)
		}
		if q != "" {
			sqlq += " AND (a.title LIKE ? OR a.content LIKE ?)"
			args = append(args, "%"+q+"%", "%"+q+"%")
		}
		sqlq += " ORDER BY a.id DESC"
		rows, err := authStore.db.Query(sqlq, args...)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		defer rows.Close()
		var out []articleDTO
		for rows.Next() {
			var a articleDTO
			var anonI int
			var nick string
			rows.Scan(&a.ID, &a.Title, &a.Category, &a.Content, &anonI, &a.Status, &a.UserID, &a.CreatedAt, &a.Cover, &nick)
			a.Anonymous = anonI == 1
			if a.Anonymous { a.UserID = 0 }
			if a.Anonymous || nick == "" { a.Author = "匿名" } else { a.Author = nick }
			// 列表不返回全文，只返回摘要
			if len(a.Content) > 120 { a.Content = a.Content[:120] + "…" }
			out = append(out, a)
		}
		apiJSON(w, 200, map[string]any{"articles": out})
	}
}

func getArticleDetail(w http.ResponseWriter, r *http.Request, id int64) {
	var a articleDTO
	var anonI int
	var nick string
	err := authStore.db.QueryRow(
		`SELECT a.id, a.title, a.category, a.content, a.is_anonymous, a.status, a.user_id, a.created_at, a.reject_reason, a.cover, COALESCE(u.nickname,'')
		 FROM articles a LEFT JOIN users u ON u.id=a.user_id WHERE a.id=?`, id).
		Scan(&a.ID, &a.Title, &a.Category, &a.Content, &anonI, &a.Status, &a.UserID, &a.CreatedAt, &a.RejectReason, &a.Cover, &nick)
	if err != nil {
		apiErr(w, 404, "文章不存在")
		return
	}
	if a.Status != "正常" {
		// 非公开状态（draft/待审/已驳回/已下架）仅作者或管理员可见
		me := currentUserID(r)
		canSee := false
		if me != 0 {
			if me == a.UserID {
				canSee = true
			} else {
				u, _ := authStore.GetByID(me)
				canSee = u != nil && u.IsAdmin == 1
			}
		}
		if !canSee {
			apiErr(w, 404, "文章不存在")
			return
		}
	}
	a.Anonymous = anonI == 1
	if a.Anonymous || nick == "" { a.Author = "匿名" } else { a.Author = nick }
	apiJSON(w, 200, map[string]any{"article": a})
}

func handleCreateArticle() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 {
			apiErr(w, 401, "请先登录")
			return
		}
		if currentUserBanned(r) {
			apiErr(w, 403, "该账号已被封禁，无法进行此操作")
			return
		}

		var body struct {
			Title     string `json:"title"`
			Category  string `json:"category"`
			Content   string `json:"content"`
			Anonymous bool   `json:"anonymous"`
			Status    string `json:"status"`
			Cover     string `json:"cover"` // J7 封面URL
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		body.Title = strings.TrimSpace(body.Title)
		body.Content = strings.TrimSpace(body.Content)
		body.Category = strings.TrimSpace(body.Category)
		body.Cover = strings.TrimSpace(body.Cover)
		// 草稿：status=draft 允许空标题/内容（先存着）；非草稿须完整且进待审
		isDraft := body.Status == "draft"
		if !isDraft && (body.Title == "" || body.Content == "") {
			apiErr(w, 400, "标题和内容不能为空")
			return
		}
		// 邮箱验证（斐波那契触发）：仅正式发布（非草稿）触发
		if !isDraft && requireEmailVerified(uid) {
			apiErr(w, 403, "请先验证邮箱后再继续发布（验证码见 /api/email/verify）")
			return
		}
		if body.Category == "" { body.Category = "生活" }
		// F3 投稿流：非草稿进"待审"，后台通过后才公开；草稿仅作者/管理员可见
		status := "待审"
		if isDraft { status = "draft" }
		_, err := authStore.db.Exec(
			"INSERT INTO articles (title,category,content,user_id,is_anonymous,status,reject_reason,cover,created_at) VALUES (?,?,?,?,?,?,?,?,?)",
			body.Title, body.Category, body.Content, uid, b2i(body.Anonymous), status, "", body.Cover, time.Now().Format("2006-01-02 15:04:05"))
		if err != nil {
			apiErr(w, 500, "投稿失败")
			return
		}
		if isDraft {
			apiJSON(w, 200, map[string]string{"message": "已保存草稿，可在个人中心继续编辑"})
		} else {
			apiJSON(w, 200, map[string]string{"message": "投稿成功，等待管理员审核后展示"})
		}
	}
}

// ---- 后台管理 ----
func handleAdminListArticles(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		// POST /api/admin/articles/{id}/status
		if r.Method == http.MethodPost {
			p := strings.TrimPrefix(r.URL.Path, "/api/admin/articles/")
			idStr := strings.TrimSuffix(p, "/status")
			id := parseID(idStr)
			if id == 0 {
				apiErr(w, 400, "无效的文章ID")
				return
			}
			var body struct {
				Status       string `json:"status"`
				RejectReason string `json:"rejectReason"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				apiErr(w, 400, "请求格式错误")
				return
			}
			if body.Status != "正常" && body.Status != "已驳回" && body.Status != "已下架" {
				apiErr(w, 400, "状态无效")
				return
			}
			reason := body.RejectReason
			if body.Status != "已驳回" {
				reason = ""
			}
			_, err := auth.db.Exec("UPDATE articles SET status=?, reject_reason=? WHERE id=?", body.Status, reason, id)
			if err != nil {
				apiErr(w, 500, "操作失败")
				return
			}
			apiJSON(w, 200, map[string]any{"ok": true, "id": id, "status": body.Status})
			return
		}
		rows, err := auth.db.Query(
			`SELECT a.id, a.title, a.category, a.content, a.is_anonymous, a.status, a.created_at,
			        a.user_id, a.reject_reason, COALESCE(u.nickname,''), COALESCE(u.email,'')
			 FROM articles a LEFT JOIN users u ON u.id=a.user_id ORDER BY a.id DESC`)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		defer rows.Close()
		type item struct {
			ID         int64  `json:"id"`
			Title      string `json:"title"`
			Category   string `json:"category"`
			Content    string `json:"content"`
			Anonymous  bool   `json:"anonymous"`
			Status       string `json:"status"`
			RejectReason string `json:"rejectReason"`
			CreatedAt    string `json:"createdAt"`
			UserID       int64  `json:"userId"`
			RealAuthor   string `json:"realAuthor"`
		}
		out := []item{}
		for rows.Next() {
			it := item{}
			var anonI int
			var nick, email string
			rows.Scan(&it.ID, &it.Title, &it.Category, &it.Content, &anonI, &it.Status, &it.CreatedAt,
				&it.UserID, &it.RejectReason, &nick, &email)
			it.Anonymous = anonI == 1
			it.RealAuthor = nick + " (" + email + ")"
			out = append(out, it)
		}
		apiJSON(w, 200, map[string]any{"total": len(out), "articles": out})
	}
}
// migrateArticleCategories 旧分类名迁移：老师→其他，生活方式→生活
func migrateArticleCategories(db *sql.DB) {
	_, _ = db.Exec("UPDATE articles SET category='其他' WHERE category='老师'")
	_, _ = db.Exec("UPDATE articles SET category='生活' WHERE category='生活方式'")
}