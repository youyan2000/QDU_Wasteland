// opinion.go — G0 意见箱：用户提交意见/建议，站长后台统一查看
// 表：
//   opinions(id, user_id, content, is_anonymous, status, created_at)
// 接口：
//   POST /api/opinion                 提交意见（登录，限频 1次/小时）
//   GET  /api/admin/opinions          后台意见列表（含 待处理/已处理）
//   POST /api/admin/opinions/{id}/status  标记 已处理 / 删除
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func migrateOpinions(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS opinions (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id       INTEGER NOT NULL,
		content       TEXT NOT NULL,
		is_anonymous  INTEGER NOT NULL DEFAULT 0,
		status        TEXT NOT NULL DEFAULT '待处理',
		created_at    TEXT NOT NULL
	)`)
}

// ---- 提交意见 ----
func handleSubmitOpinion(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 {
			apiErr(w, 401, "请先登录")
			return
		}
		if currentUserBanned(r) {
			apiErr(w, 403, "该账号已被封禁")
			return
		}
		var body struct {
			Content   string `json:"content"`
			Anonymous bool   `json:"anonymous"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		body.Content = strings.TrimSpace(body.Content)
		if body.Content == "" {
			apiErr(w, 400, "意见内容不能为空")
			return
		}
		if len(body.Content) > 2000 {
			apiErr(w, 400, "意见过长（最多2000字）")
			return
		}
		_, err := auth.db.Exec(`INSERT INTO opinions (user_id, content, is_anonymous, status, created_at) VALUES (?,?,?,?,?)`,
			uid, body.Content, b2i(body.Anonymous), "待处理", time.Now().Format("2006-01-02 15:04:05"))
		if err != nil {
			apiErr(w, 500, "提交失败")
			return
		}
		logAudit(auth.db, uid, "提交意见", substrIf(body.Content, 30))
		apiJSON(w, 200, map[string]any{"message": "意见已提交，感谢反馈", "ok": true})
	}
}

// ---- 后台：意见列表 ----
func handleAdminOpinions(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID := requireAdmin(auth, w, r)
		if adminID == 0 {
			return
		}
		// POST /api/admin/opinions/{id}/status
		if r.Method == http.MethodPost {
			p := strings.TrimPrefix(r.URL.Path, "/api/admin/opinions/")
			idStr := strings.TrimSuffix(p, "/status")
			id := parseID(idStr)
			if id == 0 {
				apiErr(w, 400, "无效意见ID")
				return
			}
			var body struct {
				Status string `json:"status"` // 已处理 / 已删除
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				apiErr(w, 400, "请求格式错误")
				return
			}
			if body.Status == "已删除" {
				_, _ = auth.db.Exec(`DELETE FROM opinions WHERE id=?`, id)
			} else if body.Status == "已处理" {
				_, _ = auth.db.Exec(`UPDATE opinions SET status=? WHERE id=?`, "已处理", id)
			} else {
				apiErr(w, 400, "状态无效")
				return
			}
			logAudit(auth.db, adminID, "处理意见", "意见ID="+strconv.FormatInt(id, 10)+" status="+body.Status)
			apiJSON(w, 200, map[string]any{"ok": true, "id": id, "status": body.Status})
			return
		}
		// 先取意见（带操作者名），分页
		page := atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		pageSize := atoi(r.URL.Query().Get("pageSize"))
		if pageSize < 1 || pageSize > 100 {
			pageSize = 30
		}
		var total int
		_ = auth.db.QueryRow(`SELECT COUNT(*) FROM opinions`).Scan(&total)
		rows, err := auth.db.Query(`SELECT id,user_id,content,is_anonymous,status,created_at FROM opinions ORDER BY id DESC LIMIT ? OFFSET ?`, pageSize, (page-1)*pageSize)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		type item struct {
			ID        int64  `json:"id"`
			UserID    int64  `json:"userId"`
			UserName  string `json:"userName"`
			Content   string `json:"content"`
			Anonymous bool   `json:"anonymous"`
			Status    string `json:"status"`
			CreatedAt string `json:"createdAt"`
		}
		var out []item
		for rows.Next() {
			it := item{}
			var anon int
			_ = rows.Scan(&it.ID, &it.UserID, &it.Content, &anon, &it.Status, &it.CreatedAt)
			it.Anonymous = anon == 1
			out = append(out, it)
		}
		rows.Close() // SQLite 单连接：先关闭再查名字
		for i := range out {
			out[i].UserName = auditOperatorName(auth.db, out[i].UserID)
		}
		apiJSON(w, 200, map[string]any{"total": total, "page": page, "pageSize": pageSize, "opinions": out})
	}
}

// substrIf 截断字符串到 n 字符（用 rune 避免中文字符截断报错）
func substrIf(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
