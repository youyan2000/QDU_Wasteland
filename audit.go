// audit.go — F6 审计日志：记录后台关键写操作，站长可追溯
// 调用方式：logAudit(db, operatorUID, action, detail)
package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// migrateAuditLogs 建审计日志表
func migrateAuditLogs(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS audit_logs (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		operator   INTEGER NOT NULL,
		action     TEXT NOT NULL,
		detail     TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`)
}

// logAudit 写一条审计记录
func logAudit(db *sql.DB, operator int64, action, detail string) {
	if db == nil {
		return
	}
	_, _ = db.Exec(`INSERT INTO audit_logs (operator, action, detail, created_at) VALUES (?,?,?,?)`,
		operator, action, detail, time.Now().Format("2006-01-02 15:04:05"))
}

// auditOperatorName 取操作者显示名（邮箱/昵称），供后台展示
func auditOperatorName(db *sql.DB, uid int64) string {
	var name string
	_ = db.QueryRow(`SELECT COALESCE(nickname, email) FROM users WHERE id=?`, uid).Scan(&name)
	return name
}

// ---- 后台：审计日志列表 ----
// GET /api/admin/audit?page=&pageSize=
func handleAdminAudit(s *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(s, w, r) == 0 {
			return
		}
		page := atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		pageSize := atoi(r.URL.Query().Get("pageSize"))
		if pageSize < 1 || pageSize > 200 {
			pageSize = 50
		}
		var total int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&total)
		// SQLite 单连接铁律：先取完 rows 并 Close，再查操作者名
		rows, err := s.db.Query(`SELECT id, operator, action, detail, created_at FROM audit_logs ORDER BY id DESC LIMIT ? OFFSET ?`, pageSize, (page-1)*pageSize)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		type item struct {
			ID        int64  `json:"id"`
			Operator  int64  `json:"operator"`
			OpName    string `json:"opName"`
			Action    string `json:"action"`
			Detail    string `json:"detail"`
			CreatedAt string `json:"createdAt"`
		}
		var out []item
		for rows.Next() {
			it := item{}
			_ = rows.Scan(&it.ID, &it.Operator, &it.Action, &it.Detail, &it.CreatedAt)
			out = append(out, it)
		}
		rows.Close()
		// 批量取操作者名（避免 N+1 查询）：一次 IN 查询拿全部昵称/邮箱
		if len(out) > 0 {
			seen := map[int64]string{}
			var ids []string
			for i := range out {
				if _, ok := seen[out[i].Operator]; !ok {
					seen[out[i].Operator] = ""
					ids = append(ids, strconv.FormatInt(out[i].Operator, 10))
				}
			}
			rows2, err := s.db.Query(
				`SELECT id, COALESCE(nickname, email) FROM users WHERE id IN (`+strings.Join(ids, ",")+`)`)
			if err == nil {
				for rows2.Next() {
					var uid int64
					var name string
					_ = rows2.Scan(&uid, &name)
					seen[uid] = name
				}
				rows2.Close()
			}
			for i := range out {
				out[i].OpName = seen[out[i].Operator]
			}
		}
		apiJSON(w, 200, map[string]any{"total": total, "page": page, "pageSize": pageSize, "logs": out})
	}
}
