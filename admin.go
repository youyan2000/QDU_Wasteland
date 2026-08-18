// admin.go — 后台管理接口 (V5)
// 提供：
//   - 站长播种（环境变量 ADMIN_EMAIL / ADMIN_PASSWORD，仅当无管理员时生效）
//   - /api/admin/* 鉴权中间件（登录 + is_admin=1）
//   - 数据总览 /api/admin/dashboard
//   - 用户管理 /api/admin/users  (+ 设管理员 /api/admin/users/{id}/admin)
//   - 举报中心雏形 /api/admin/reports  (+ 处理 /api/admin/reports/{id}/resolve)
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// ---- 后台鉴权中间件 ----
// allowUserID 返回已登录且为管理员的 userID；否则写 403 并返回 0
func requireAdmin(s *AuthStore, w http.ResponseWriter, r *http.Request) int64 {
	uid := currentUserID(r)
	if uid == 0 {
		apiErr(w, 401, "请先登录")
		return 0
	}
	u, err := s.GetByID(uid)
	if err != nil || u == nil || u.IsAdmin != 1 {
		apiErr(w, 403, "无权限：仅管理员可访问后台")
		return 0
	}
	return uid
}

// ---- 站长播种（幂等：只有没有任何管理员时才执行）----
func seedAdmin(s *AuthStore) {
	if s == nil {
		return
	}
	users, err := s.GetAllUsers()
	if err != nil {
		return
	}
	for _, u := range users {
		if u.IsAdmin == 1 {
			return // 已有管理员，环境变量不再生效
		}
	}
	email := strings.TrimSpace(strings.ToLower(os.Getenv("ADMIN_EMAIL")))
	pass := os.Getenv("ADMIN_PASSWORD")
	if email == "" || pass == "" {
		return
	}
	// 复用现有注册逻辑：若邮箱已存在则置为管理员，否则创建
	if exist, _ := s.GetByEmail(email); exist != nil {
		s.SetAdmin(exist.ID, 1)
		return
	}
	u, err := s.CreateUser(email, "admin", pass, "站长", "", "")
	if err != nil {
		return
	}
	s.SetAdmin(u.ID, 1)
}

// ---- /api/admin/dashboard 数据总览 ----
func handleAdminDashboard(s *AuthStore, db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(s, w, r) == 0 {
			return
		}
		userCount := countRows(s, "users")
		postCount := countRows(s, "posts")
		fileCount := countRows(s, "files")
		reviewCount := countRows(s, "reviews")
		var reportPending int
		s.db.QueryRow("SELECT COUNT(*) FROM reports WHERE status='待处理'").Scan(&reportPending)
		apiJSON(w, 200, map[string]any{
			"userCount":    userCount,
			"postCount":    postCount,
			"fileCount":    fileCount,
			"reviewCount":  reviewCount,
			"reportPending": reportPending,
			"courseCount":  len(db.Courses),
			"offeringCount": len(db.Offerings),
			"collegeCount": len(db.CollegeSet),
		})
	}
}

// countRows 简单计数辅助
func countRows(s *AuthStore, table string) int {
	if s == nil || s.db == nil {
		return 0
	}
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
	return n
}

// ---- /api/admin/users 用户管理（列表 + 设管理员）----
func handleAdminUsers(s *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(s, w, r) == 0 {
			return
		}
		// PATCH /api/admin/users/{id}/admin   body: {"isAdmin":0|1}
		if r.Method == http.MethodPatch {
			p := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
			idStr := strings.TrimSuffix(p, "/admin")
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				apiErr(w, 400, "无效的用户ID")
				return
			}
			var body struct {
				IsAdmin int `json:"isAdmin"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				apiErr(w, 400, "请求格式错误")
				return
			}
			v := 0
			if body.IsAdmin != 0 {
				v = 1
			}
			if err := s.SetAdmin(id, v); err != nil {
				apiErr(w, 500, "操作失败")
				return
			}
			apiJSON(w, 200, map[string]any{"ok": true, "id": id, "isAdmin": v})
			return
		}
		// GET /api/admin/users 列表（支持分页 page/pageSize）
		page := atoi(r.URL.Query().Get("page"))
		if page < 1 { page = 1 }
		pageSize := atoi(r.URL.Query().Get("pageSize"))
		if pageSize < 1 || pageSize > 200 { pageSize = 50 }
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		where := ""
		wargs := []any{}
		if q != "" {
			where = " WHERE email LIKE ? OR nickname LIKE ? OR college LIKE ? OR major LIKE ?"
			wargs = append(wargs, "%"+q+"%", "%"+q+"%", "%"+q+"%", "%"+q+"%")
		}
		var total int
		s.db.QueryRow("SELECT COUNT(*) FROM users"+where, wargs...).Scan(&total)
		queryArgs := append(append([]any{}, wargs...), pageSize, (page-1)*pageSize)
		rows, err := s.db.Query("SELECT id, email, nickname, college, major, is_admin, banned, COALESCE(muted,0), created_at FROM users"+where+" ORDER BY id DESC LIMIT ? OFFSET ?", queryArgs...)
		if err != nil {
			apiErr(w, 500, "读取失败")
			return
		}
		defer rows.Close()
		var out []SessionInfo
		for rows.Next() {
			si := SessionInfo{}
			var created string
			var mutedI int
			if err := rows.Scan(&si.ID, &si.Email, &si.Nickname, &si.College, &si.Major, &si.IsAdmin, &si.Banned, &mutedI, &created); err != nil {
				continue
			}
			si.Muted = mutedI
			si.CreatedAt = created
			out = append(out, si)
		}
		apiJSON(w, 200, map[string]any{"total": total, "page": page, "pageSize": pageSize, "users": out})
	}
}

// ---- 举报中心雏形 ----
// 说明：前端论坛暂无举报提交按钮（V8 再做），先在后台提供 reports 表 + 可查询接口。
// GET  /api/admin/reports            举报列表
// PATCH /api/admin/reports/{id}/resolve  body: {"action":"ignore"|"remove"}
func handleAdminReports(s *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID := requireAdmin(s, w, r)
		if adminID == 0 {
			return
		}
		if r.Method == http.MethodPatch {
			idStr := strings.TrimPrefix(r.URL.Path, "/api/admin/reports/")
			idStr = strings.TrimSuffix(idStr, "/resolve")
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				apiErr(w, 400, "无效的举报ID")
				return
			}
			var body struct {
				Action string `json:"action"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				apiErr(w, 400, "请求格式错误")
				return
			}
			status := "已忽略"
			if body.Action == "remove" {
				status = "已下架"
			}
			_, err = s.db.Exec("UPDATE reports SET status=?, resolved_by=? WHERE id=?",
				status, adminID, id)
			if err != nil {
				apiErr(w, 500, "处理失败")
				return
			}
			logAudit(s.db, adminID, "处理举报", "举报ID="+strconv.FormatInt(id, 10)+" action="+body.Action)
			apiJSON(w, 200, map[string]any{"ok": true, "id": id, "status": status})
			return
		}
		// 未处理优先，再按时间倒序；LIMIT 500 防全量加载（举报多时性能）
		// 附带目标作者昵称/ID、目标内容预览（评论附带所属帖子ID），便于后台查看/定位
		rows, err := s.db.Query(`SELECT r.id, r.target_type, r.target_id, r.reporter_id, r.reason, r.status, r.created_at,
			COALESCE(ra.nickname,''), COALESCE(ra.id,0),
			CASE r.target_type
				WHEN 'post' THEN (SELECT title FROM posts WHERE id=CAST(r.target_id AS INTEGER))
				WHEN 'article' THEN (SELECT title FROM articles WHERE id=CAST(r.target_id AS INTEGER))
				WHEN 'file' THEN (SELECT title FROM files WHERE id=CAST(r.target_id AS INTEGER))
				WHEN 'review' THEN (SELECT course_code FROM reviews WHERE id=CAST(r.target_id AS INTEGER))
				WHEN 'comment' THEN (SELECT content FROM comments WHERE id=CAST(r.target_id AS INTEGER))
				ELSE '' END,
			CASE r.target_type
				WHEN 'comment' THEN (SELECT post_id FROM comments WHERE id=CAST(r.target_id AS INTEGER))
				ELSE 0 END
		 FROM reports r LEFT JOIN users ra ON ra.id=r.target_author_id
		 ORDER BY CASE r.status WHEN '待处理' THEN 0 ELSE 1 END, r.id DESC LIMIT 500`)
		if err != nil {
			apiErr(w, 500, "读取失败")
			return
		}
		defer rows.Close()
		type item struct {
			ID             int64  `json:"id"`
			TargetType     string `json:"targetType"`
			TargetID       string `json:"targetId"`
			ReporterID     int64  `json:"reporterId"`
			Reason         string `json:"reason"`
			Status         string `json:"status"`
			CreatedAt      string `json:"createdAt"`
			TargetAuthor   string `json:"targetAuthor"`
			TargetAuthorID int64  `json:"targetAuthorId"`
			TargetPreview  string `json:"targetPreview"`
			CommentPostID  int64  `json:"commentPostId"`
		}
		out := []item{}
		for rows.Next() {
			it := item{}
			var targetID string
			rows.Scan(&it.ID, &it.TargetType, &targetID, &it.ReporterID, &it.Reason, &it.Status, &it.CreatedAt,
				&it.TargetAuthor, &it.TargetAuthorID, &it.TargetPreview, &it.CommentPostID)
			it.TargetID = targetID
			out = append(out, it)
		}
		// 搜索过滤（内存过滤，最多 500 条）：匹配原因/被举报人/目标预览
		if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
			filtered := out[:0]
			for _, it := range out {
				if strings.Contains(it.Reason, q) || strings.Contains(it.TargetAuthor, q) || strings.Contains(it.TargetPreview, q) || strings.Contains(it.TargetID, q) {
					filtered = append(filtered, it)
				}
			}
			out = filtered
		}
		// 分页（内存切片）
		total := len(out)
		page := atoi(r.URL.Query().Get("page"))
		if page < 1 { page = 1 }
		pageSize := atoi(r.URL.Query().Get("pageSize"))
		if pageSize < 1 || pageSize > 100 { pageSize = 30 }
		start := (page - 1) * pageSize
		if start > len(out) {
			out = []item{}
		} else {
			end := start + pageSize
			if end > len(out) { end = len(out) }
			out = out[start:end]
		}
		apiJSON(w, 200, map[string]any{"total": total, "page": page, "pageSize": pageSize, "reports": out})
	}
}

// handleAdminUserBan 封禁/解封用户（管理员）
// PATCH /api/admin/users/{id}/ban  body: {"banned":0|1}
func handleAdminUserBan(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 { return }
		p := strings.TrimPrefix(r.URL.Path, "/api/admin/userban/")
		idStr := strings.TrimSuffix(p, "/ban")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id == 0 { apiErr(w, 400, "无效用户"); return }
		if id == currentUserID(r) { apiErr(w, 400, "不能封禁自己"); return }
		var body struct{ Banned int `json:"banned"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil { apiErr(w, 400, "请求格式错误"); return }
		v := 0
		if body.Banned != 0 { v = 1 }
		_, err = auth.db.Exec("UPDATE users SET banned=? WHERE id=?", v, id)
		if err != nil { apiErr(w, 500, "操作失败"); return }
		apiJSON(w, 200, map[string]any{"ok": true, "id": id, "banned": v})
	}
}

// ---- F1 内容审核队列 ----
// 敏感词命中/举报拦截的内容以 status='待复核' 落库（不出现在前台），站长在此审核：
//   GET  /api/admin/review                    待复核 + 已下架 内容统一队列
//   POST /api/admin/review/{type}/{id}         body: {"action":"approve"|"remove"|"restore"}
type reviewItem struct {
	Type     string `json:"type"`
	ID       int64  `json:"id"`
	Ref      string `json:"ref"`     // 帖子广场/课程码/分类等定位信息
	Preview  string `json:"preview"` // 标题或内容摘要
	Author   string `json:"author"`
	Status   string `json:"status"`
	Created  string `json:"createdAt"`
	ReviewID int64  `json:"reviewId"` // 统一序号
}

func handleAdminReview(s *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(s, w, r) == 0 {
			return
		}
		// POST /api/admin/review/{type}/{id}
		if r.Method == http.MethodPost {
			p := strings.TrimPrefix(r.URL.Path, "/api/admin/review/")
			parts := strings.Split(p, "/")
			if len(parts) < 2 {
				apiErr(w, 400, "无效请求")
				return
			}
			kind := parts[0]
			id, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil || id == 0 {
				apiErr(w, 400, "无效ID")
				return
			}
			var body struct {
				Action string `json:"action"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				apiErr(w, 400, "请求格式错误")
				return
			}
			next := "正常"
			switch body.Action {
			case "approve", "restore":
				next = "正常"
			case "remove":
				next = "已下架"
			default:
				apiErr(w, 400, "无效操作")
				return
			}
			var table, idCol string
			switch kind {
			case "post":
				table, idCol = "posts", "id"
			case "article":
				table, idCol = "articles", "id"
			case "file":
				table, idCol = "files", "id"
			case "review":
				table, idCol = "reviews", "id"
			default:
				apiErr(w, 400, "无效类型")
				return
			}
			if _, err := s.db.Exec("UPDATE "+table+" SET status=? WHERE "+idCol+"=?", next, id); err != nil {
				apiErr(w, 500, "操作失败")
				return
			}
			apiJSON(w, 200, map[string]any{"ok": true, "type": kind, "id": id, "status": next})
			return
		}
		// GET 队列（LEFT JOIN 直接取昵称，避免单连接二次查询死锁）
		out := []reviewItem{}
		seq := int64(0)
		collect := func(kind, sql string) {
			rows, err := s.db.Query(sql)
			if err != nil {
				return
			}
			defer rows.Close()
			for rows.Next() {
				var it reviewItem
				var anonI int
				var nick, uname, ref, created string
				if err := rows.Scan(&it.ID, &ref, &it.Preview, &anonI, &it.Status, &created, &nick, &uname); err != nil {
					continue
				}
				it.Type = kind
				it.Created = created
				it.ReviewID = seq
				seq++
				if anonI == 1 {
					it.Author = "匿名"
				} else if nick != "" {
					it.Author = nick
				} else if uname != "" {
					it.Author = uname
				} else {
					it.Author = "系统"
				}
				out = append(out, it)
			}
		}
		collect("post", `SELECT p.id, p.forum, p.title, p.is_anonymous, p.status, p.created_at, COALESCE(u.nickname,''), COALESCE(u.username,'')
			FROM posts p LEFT JOIN users u ON u.id=p.user_id WHERE p.status IN ('待复核','已下架') ORDER BY p.id DESC`)
		collect("article", `SELECT a.id, a.category, a.title, a.is_anonymous, a.status, a.created_at, COALESCE(u.nickname,''), COALESCE(u.username,'')
			FROM articles a LEFT JOIN users u ON u.id=a.user_id WHERE a.status IN ('待复核','已下架') ORDER BY a.id DESC`)
		collect("file", `SELECT f.id, f.course_code, f.title, f.is_anonymous, f.status, f.created_at, COALESCE(u.nickname,''), COALESCE(u.username,'')
			FROM files f LEFT JOIN users u ON u.id=f.uploader_id WHERE f.status IN ('待复核','已下架') ORDER BY f.id DESC`)
		collect("review", `SELECT r.id, r.course_code, r.content, r.is_anonymous, r.status, r.created_at, COALESCE(u.nickname,''), COALESCE(u.username,'')
			FROM reviews r LEFT JOIN users u ON u.id=r.user_id WHERE r.status IN ('待复核','已下架') ORDER BY r.id DESC`)
		apiJSON(w, 200, map[string]any{"total": len(out), "items": out})
	}
}

// userNickname 取昵称（不存在回落为 id）
func userNickname(s *AuthStore, uid int64) string {
	if uid <= 0 {
		return "系统"
	}
	if s == nil {
		return strconv.FormatInt(uid, 10)
	}
	u, err := s.GetByID(uid)
	if err != nil || u == nil {
		return strconv.FormatInt(uid, 10)
	}
	if u.Nickname != "" {
		return u.Nickname
	}
	return u.Username
}