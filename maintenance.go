// maintenance.go — F7 维护模式：站点开关，开启时前台展示"维护中"
// 管理员仍可访问后台（维护只影响普通访问者）
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// migrateMaintenance 建维护开关表（单行）
func migrateMaintenance(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS maintenance (
		id      INTEGER PRIMARY KEY CHECK (id=1),
		enabled INTEGER NOT NULL DEFAULT 0,
		msg     TEXT NOT NULL DEFAULT ''
	)`)
	// 确保有默认行
	var cnt int
	_ = db.QueryRow(`SELECT COUNT(*) FROM maintenance`).Scan(&cnt)
	if cnt == 0 {
		_, _ = db.Exec(`INSERT INTO maintenance (id, enabled, msg) VALUES (1,0,'')`)
	}
}

// maintenanceEnabled 查询维护开关
func maintenanceEnabled(sdb *sql.DB) bool {
	var enabled int
	_ = sdb.QueryRow(`SELECT enabled FROM maintenance WHERE id=1`).Scan(&enabled)
	return enabled == 1
}

// maintenanceMsg 查询维护提示
func maintenanceMsg(sdb *sql.DB) string {
	var msg string
	_ = sdb.QueryRow(`SELECT msg FROM maintenance WHERE id=1`).Scan(&msg)
	return msg
}

// maintenanceInterceptor 中间件：维护开启时，普通访问者返回维护页（API 除外）
func maintenanceInterceptor(sdb *sql.DB, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 未开启 → 正常
		if !maintenanceEnabled(sdb) {
			h.ServeHTTP(w, r)
			return
		}
		// 已开启：管理员 / API / 静态资源放行，普通页面拦截
		if isAdminRequest(r, sdb) || isStaticOrAPI(r) {
			h.ServeHTTP(w, r)
			return
		}
		// 维护页
		msg := maintenanceMsg(sdb)
		if msg == "" {
			msg = "站点维护中，请稍后再来。"
		}
		writeMaintenancePage(w, msg)
	})
}

// isAdminRequest 判断当前请求是否为管理员
func isAdminRequest(r *http.Request, sdb *sql.DB) bool {
	uid := currentUserID(r)
	if uid == 0 {
		return false
	}
	var isAdmin int
	_ = sdb.QueryRow(`SELECT is_admin FROM users WHERE id=?`, uid).Scan(&isAdmin)
	return isAdmin == 1
}

// writeMaintenancePage 返回维护提示页
func writeMaintenancePage(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte(`<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><title>维护中</title>
<style>body{font-family:-apple-system,'PingFang SC',sans-serif;background:#faf9f6;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.card{text-align:center;max-width:420px;padding:40px;background:#fff;border:1px solid #eee;border-radius:16px;box-shadow:0 8px 30px rgba(0,0,0,.06)}
h1{font-size:1.6rem;margin:16px 0 8px}p{color:#666;line-height:1.7}</style></head>
<body><div class="card"><div style="font-size:2.4rem">🛠</div><h1>维护中</h1><p>` + msg + `</p></div></body></html>`))
}

// ---- 后台：维护开关 ----
// GET  /api/admin/maintenance  查状态
// POST /api/admin/maintenance  body:{enabled, msg}
func handleAdminMaintenance(s *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(s, w, r) == 0 {
			return
		}
		if r.Method == http.MethodPost {
			var body struct {
				Enabled int    `json:"enabled"`
				Msg     string `json:"msg"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				apiErr(w, 400, "请求格式错误")
				return
			}
			var enabled int
			if body.Enabled != 0 {
				enabled = 1
			}
			_, _ = s.db.Exec(`UPDATE maintenance SET enabled=?, msg=? WHERE id=1`, enabled, body.Msg)
			logAudit(s.db, currentUserID(r), "维护模式", "enabled="+itoaBody(body.Enabled))
			apiJSON(w, 200, map[string]any{"ok": true, "enabled": enabled, "msg": body.Msg})
			return
		}
		apiJSON(w, 200, map[string]any{"enabled": maintenanceEnabled(s.db), "msg": maintenanceMsg(s.db)})
	}
}

func itoaBody(i int) string { return strconv.Itoa(i) }

// isStaticOrAPI 判断是否 API 或静态资源（维护时放行）
func isStaticOrAPI(r *http.Request) bool {
	p := r.URL.Path
	if len(p) >= 5 && p[:5] == "/api/" {
		return true
	}
	for _, ext := range []string{".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".woff", ".woff2"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}
