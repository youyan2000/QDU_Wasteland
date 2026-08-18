// admin_posts.go — 后台帖子管理
// GET  /api/admin/posts?page=&pageSize=&q=   帖子列表（含真实作者/板块/状态，可搜索）
// POST /api/admin/posts/{id}/status          下架/恢复  body:{"status":"正常"|"已下架"}
package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// handleAdminListPosts 后台帖子列表（分页 + 搜索）
func handleAdminListPosts(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		// 单条状态：POST /api/admin/posts/{id}/status
		if r.Method == http.MethodPost {
			p := strings.TrimPrefix(r.URL.Path, "/api/admin/posts/")
			idStr := strings.TrimSuffix(p, "/status")
			id := parseID(idStr)
			if id == 0 {
				apiErr(w, 400, "无效的帖子ID")
				return
			}
			var body struct{ Status string `json:"status"` }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				apiErr(w, 400, "请求格式错误")
				return
			}
			if body.Status != "正常" && body.Status != "已下架" {
				apiErr(w, 400, "状态无效")
				return
			}
			_, err := auth.db.Exec("UPDATE posts SET status=? WHERE id=?", body.Status, id)
			if err != nil {
				apiErr(w, 500, "操作失败")
				return
			}
			logAudit(auth.db, currentUserID(r), "处理帖子", "帖子ID="+strconv.FormatInt(id, 10)+" status="+body.Status)
			apiJSON(w, 200, map[string]any{"ok": true, "id": id, "status": body.Status})
			return
		}
		// 列表
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		if page < 1 { page = 1 }
		if pageSize < 1 || pageSize > 100 { pageSize = 50 }
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		where := " WHERE 1=1"
		args := []any{}
		if q != "" {
			where += " AND (p.title LIKE ? OR p.content LIKE ? OR p.forum LIKE ? OR u.nickname LIKE ? OR u.email LIKE ?)"
			args = append(args, "%"+q+"%", "%"+q+"%", "%"+q+"%", "%"+q+"%", "%"+q+"%")
		}
		var total int
		auth.db.QueryRow(`SELECT COUNT(*) FROM posts p LEFT JOIN users u ON u.id=p.user_id`+where, args...).Scan(&total)
		args = append(args, pageSize, (page-1)*pageSize)
		rows, err := auth.db.Query(
			`SELECT p.id, p.forum, p.title, p.content, p.is_anonymous, p.status, p.created_at,
			        p.user_id, p.likes, COALESCE(u.nickname,''), COALESCE(u.email,'')
			 FROM posts p LEFT JOIN users u ON u.id=p.user_id`+where+` ORDER BY p.id DESC LIMIT ? OFFSET ?`, args...)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		defer rows.Close()
		type item struct {
			ID         int64  `json:"id"`
			Forum      string `json:"forum"`
			Title      string `json:"title"`
			Content    string `json:"content"`
			Anonymous  bool   `json:"anonymous"`
			Status     string `json:"status"`
			CreatedAt  string `json:"createdAt"`
			UserID     int64  `json:"userId"`
			Likes      int    `json:"likes"`
			RealAuthor string `json:"realAuthor"`
		}
		out := []item{}
		for rows.Next() {
			it := item{}
			var anonI int
			var nick, email string
			rows.Scan(&it.ID, &it.Forum, &it.Title, &it.Content, &anonI, &it.Status, &it.CreatedAt,
				&it.UserID, &it.Likes, &nick, &email)
			it.Anonymous = anonI == 1
			if nick == "" { nick = "（已注销用户）" }
			it.RealAuthor = nick + " (" + email + ")"
			if len(it.Content) > 80 { it.Content = it.Content[:80] + "…" }
			out = append(out, it)
		}
		apiJSON(w, 200, map[string]any{"total": total, "posts": out})
	}
}
