// announcement.go — V13 (D3) 站长公告
// GET   /api/announcement            最新公告（公开）
// GET   /api/admin/announcement      公告列表（管理员，含展示状态）
// POST  /api/admin/announcement      发布公告（管理员，content）
// DELETE /api/admin/announcement     清空公告（管理员）
// DELETE /api/admin/announcement/{id} 删除单条公告（管理员）
package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handleGetAnnouncement 返回最新公告（公开）
func handleGetAnnouncement() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var content, created string
		var id int64
		err := authStore.db.QueryRow("SELECT id, content, created_at FROM announcements WHERE status='展示中' ORDER BY id DESC LIMIT 1").Scan(&id, &content, &created)
		if err != nil {
			apiJSON(w, 200, map[string]any{"announcement": nil})
			return
		}
		apiJSON(w, 200, map[string]any{"announcement": map[string]any{"id": id, "content": content, "createdAt": created}})
	}
}

// handleAdminAnnouncement 后台公告管理（管理员）
// 同一时间只向全站展示最新一条；历史公告保留在列表中，可单独删除。
func handleAdminAnnouncement() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(authStore, w, r) == 0 {
			return
		}
		// 单条操作：DELETE /api/admin/announcement/{id} 删除；POST /api/admin/announcement/{id}/status 下架/恢复
		rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/announcement"), "/")
		if rest != "" {
			idStr := strings.TrimSuffix(rest, "/status")
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil || id <= 0 {
				apiErr(w, 400, "参数错误")
				return
			}
			if r.Method == http.MethodDelete {
				_, err = authStore.db.Exec("DELETE FROM announcements WHERE id=?", id)
				if err != nil {
					apiErr(w, 500, "删除失败")
					return
				}
				apiJSON(w, 200, map[string]string{"message": "公告已删除"})
				return
			}
			if r.Method == http.MethodPost {
				var body struct{ Status string `json:"status"` }
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					apiErr(w, 400, "请求格式错误")
					return
				}
				if body.Status != "展示中" && body.Status != "已下架" {
					apiErr(w, 400, "状态无效")
					return
				}
				_, err = authStore.db.Exec("UPDATE announcements SET status=? WHERE id=?", body.Status, id)
				if err != nil {
					apiErr(w, 500, "操作失败")
					return
				}
				apiJSON(w, 200, map[string]string{"message": "公告已" + map[string]string{"展示中": "恢复展示", "已下架": "下架"}[body.Status]})
				return
			}
			apiErr(w, 405, "不支持的方法")
			return
		}
		switch r.Method {
		case http.MethodGet:
			page := atoi(r.URL.Query().Get("page"))
			if page < 1 { page = 1 }
			pageSize := atoi(r.URL.Query().Get("pageSize"))
			if pageSize < 1 || pageSize > 100 { pageSize = 30 }
			var total int
			authStore.db.QueryRow("SELECT COUNT(*) FROM announcements").Scan(&total)
			rows, err := authStore.db.Query("SELECT id, content, created_at, status FROM announcements ORDER BY id DESC LIMIT ? OFFSET ?", pageSize, (page-1)*pageSize)
			if err != nil {
				apiErr(w, 500, "查询失败")
				return
			}
			defer rows.Close()
			type ann struct {
				ID        int64  `json:"id"`
				Content   string `json:"content"`
				CreatedAt string `json:"createdAt"`
				Status    string `json:"status"` // 展示中 / 已下架
			}
			var out []ann
			for rows.Next() {
				a := ann{}
				rows.Scan(&a.ID, &a.Content, &a.CreatedAt, &a.Status)
				out = append(out, a)
			}
			apiJSON(w, 200, map[string]any{"total": total, "page": page, "pageSize": pageSize, "announcements": out})
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