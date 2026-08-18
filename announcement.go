// announcement.go — V13 (D3) 站长公告
// GET  /api/announcement        最新公告（公开）
// POST /api/admin/announcement  发布公告（管理员，content）
// DELETE /api/admin/announcement  清空公告（管理员）
package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// handleGetAnnouncement 返回最新公告（公开）
func handleGetAnnouncement() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var content, created string
		var id int64
		err := authStore.db.QueryRow("SELECT id, content, created_at FROM announcements ORDER BY id DESC LIMIT 1").Scan(&id, &content, &created)
		if err != nil {
			apiJSON(w, 200, map[string]any{"announcement": nil})
			return
		}
		apiJSON(w, 200, map[string]any{"announcement": map[string]any{"id": id, "content": content, "createdAt": created}})
	}
}

// handleAdminAnnouncement 后台公告管理（管理员）
func handleAdminAnnouncement() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(authStore, w, r) == 0 {
			return
		}
		switch r.Method {
		case http.MethodPost:
			var body struct{ Content string `json:"content"` }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				apiErr(w, 400, "请求格式错误")
				return
			}
			content := strings.TrimSpace(body.Content)
			if content == "" {
				apiErr(w, 400, "公告内容不能为空")
				return
			}
			if len([]rune(content)) > 500 {
				apiErr(w, 400, "公告内容过长")
				return
			}
			_, err := authStore.db.Exec("INSERT INTO announcements (content, created_at) VALUES (?,?)",
				content, time.Now().Format("2006-01-02 15:04:05"))
			if err != nil {
				apiErr(w, 500, "发布失败")
				return
			}
			apiJSON(w, 200, map[string]string{"message": "公告已发布"})
		case http.MethodDelete:
			_, _ = authStore.db.Exec("DELETE FROM announcements")
			apiJSON(w, 200, map[string]string{"message": "公告已清空"})
		default:
			apiErr(w, 405, "不支持的方法")
		}
	}
}