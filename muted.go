// muted.go — 禁言（限制特定账号发布内容，但可登录浏览）
// users.muted=1 时：不能发帖/评论/评价/上传/发文章，但可正常登录浏览。
// 后台：PATCH /api/admin/userban/{id}/mute  body:{muted:0|1}
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// migrateMuted 确保 users 表含 muted 列
func migrateMuted(db *sql.DB) {
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='muted'`).Scan(&cnt); err != nil {
		return
	}
	if cnt == 0 {
		_, _ = db.Exec("ALTER TABLE users ADD COLUMN muted INTEGER NOT NULL DEFAULT 0")
	}
}

// currentUserMuted 当前登录用户是否被禁言
func currentUserMuted(r *http.Request) bool {
	uid := currentUserID(r)
	if uid == 0 { return false }
	var m int
	authStore.db.QueryRow("SELECT COALESCE(muted,0) FROM users WHERE id=?", uid).Scan(&m)
	return m == 1
}

// handleAdminUserMute 后台禁言/解除（管理员）
// PATCH /api/admin/userban/{id}/mute  body:{"muted":0|1}
func handleAdminUserMute(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 { return }
		p := strings.TrimPrefix(r.URL.Path, "/api/admin/userban/")
		p = strings.TrimSuffix(p, "/mute")
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil || id == 0 { apiErr(w, 400, "无效用户"); return }
		if id == currentUserID(r) { apiErr(w, 400, "不能禁言自己"); return }
		var body struct{ Muted int `json:"muted"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil { apiErr(w, 400, "请求格式错误"); return }
		v := 0
		if body.Muted != 0 { v = 1 }
		_, err = auth.db.Exec("UPDATE users SET muted=? WHERE id=?", v, id)
		if err != nil { apiErr(w, 500, "操作失败"); return }
		logAudit(auth.db, currentUserID(r), "禁言用户", "用户ID="+strconv.FormatInt(id, 10)+" muted="+strconv.Itoa(v))
		apiJSON(w, 200, map[string]any{"ok": true, "id": id, "muted": v})
	}
}
