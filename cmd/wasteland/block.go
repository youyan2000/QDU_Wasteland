// block.go — G5 �ɪ�˺���Ψ����˿/֪ͨ��
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func migrateBlocklist(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS blocklist (
		blocker_id INTEGER NOT NULL,
		blocked_id INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY (blocker_id, blocked_id)
	)`)
}
func isBlocked(sdb *sql.DB, blocker, target int64) bool {
	if blocker <= 0 || target <= 0 { return false }
	var n int
	_ = sdb.QueryRow(`SELECT COUNT(*) FROM blocklist WHERE blocker_id=? AND blocked_id=?`, blocker, target).Scan(&n)
	return n > 0
}
func handleToggleBlock(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 { apiErr(w, 401, "请先登录"); return }
		var body struct { ID int64 `json:"id"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil { apiErr(w, 400, "请求格式错误"); return }
		if body.ID == 0 || body.ID == uid { apiErr(w, 400, "无效的用户"); return }
		var cnt int
		_ = auth.db.QueryRow(`SELECT COUNT(*) FROM blocklist WHERE blocker_id=? AND blocked_id=?`, uid, body.ID).Scan(&cnt)
		if cnt > 0 {
			_, _ = auth.db.Exec(`DELETE FROM blocklist WHERE blocker_id=? AND blocked_id=?`, uid, body.ID)
			logAudit(auth.db, uid, "取消拉黑", "target="+strconv.FormatInt(body.ID,10))
			apiJSON(w, 200, map[string]any{"ok": true, "blocked": false})
			return
		}
		_, _ = auth.db.Exec(`INSERT INTO blocklist (blocker_id,blocked_id,created_at) VALUES (?,?,?)`, uid, body.ID, time.Now().Format("2006-01-02 15:04:05"))
		logAudit(auth.db, uid, "拉黑用户", "target="+strconv.FormatInt(body.ID,10))
		apiJSON(w, 200, map[string]any{"ok": true, "blocked": true})
	}
}
func handleBlockList(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 { apiErr(w, 401, "请先登录"); return }
		rows, err := auth.db.Query(`SELECT b.blocked_id, COALESCE(u.nickname,'') FROM blocklist b LEFT JOIN users u ON u.id=b.blocked_id WHERE b.blocker_id=? ORDER BY b.created_at DESC`, uid)
		if err != nil { apiErr(w, 500, "查询失败"); return }
		type item struct { ID int64 `json:"id"`; Nickname string `json:"nickname"` }
		var out []item
		for rows.Next() { it := item{}; _ = rows.Scan(&it.ID, &it.Nickname); out = append(out, it) }
		rows.Close()
		apiJSON(w, 200, map[string]any{"blocked": out})
	}
}
