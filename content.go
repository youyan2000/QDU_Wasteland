// content.go — 课程资料(文件)与评价 (V3)
// files 表存资料记录，文件本体存 uploads/ 目录
// reviews 表存课程评价（匿名可选）
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ---- 文件/资料 ----

// saveFile 把上传的文件存到磁盘 + 数据库
func saveFile(uploaderID int64, courseCode, title, category, teacher, semester, description string, fileOrigName string, data []byte, isAnonymous bool, status string) error {
	if err := os.MkdirAll("uploads", 0o755); err != nil {
		return err
	}
	stored := fmt.Sprintf("f_%d_%d", time.Now().UnixNano(), uploaderID)
	ext := filepath.Ext(fileOrigName)
	path := filepath.Join("uploads", stored+ext)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	_, err := authStore.db.Exec(
		"INSERT INTO files (course_code, title, category, teacher, semester, description, file_name, stored_name, size, uploader_id, is_anonymous, status, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)",
		courseCode, title, category, teacher, semester, description, fileOrigName, stored+ext, len(data), uploaderID, b2i(isAnonymous), status, time.Now().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		os.Remove(path)
	}
	return err
}

// listFiles 列出某课程的资料
type fileDTO struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Teacher     string `json:"teacher"`
	Semester    string `json:"semester"`
	Description string `json:"description"`
	FileName    string `json:"fileName"`
	Size        int64  `json:"size"`
	Uploader    string `json:"uploader"`
	Anonymous   bool   `json:"anonymous"`
	DownloadURL string `json:"downloadUrl"`
	CreatedAt   string `json:"createdAt"`
}

func listFiles(courseCode string) ([]fileDTO, error) {
	rows, err := authStore.db.Query(
		`SELECT f.id, f.title, f.category, f.teacher, f.semester, f.description, f.file_name, f.size, f.uploader_id, f.is_anonymous, f.created_at, COALESCE(u.nickname,'匿名')
		 FROM files f LEFT JOIN users u ON u.id = f.uploader_id
		 WHERE f.course_code = ? AND f.status = '正常' AND COALESCE(u.banned,0)=0 ORDER BY f.id DESC`, courseCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fileDTO
	for rows.Next() {
		var f fileDTO
		var uid int64
		var anonI int
		var nickname string
		if err := rows.Scan(&f.ID, &f.Title, &f.Category, &f.Teacher, &f.Semester, &f.Description, &f.FileName, &f.Size, &uid, &anonI, &f.CreatedAt, &nickname); err != nil {
			continue
		}
		f.Anonymous = anonI == 1
		if f.Anonymous || nickname == "" {
			f.Uploader = "匿名"
		} else {
			f.Uploader = nickname
		}
		f.DownloadURL = fmt.Sprintf("/api/files/%d/download", f.ID)
		out = append(out, f)
	}
	return out, nil
}

// ---- 列出某课程的资料 ----
func handleListFilesAPI(_ *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			apiErr(w, 400, "缺少课程")
			return
		}
		files, err := listFiles(code)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		apiJSON(w, 200, map[string]any{"files": files})
	}
}

// ---- 上传接口 ----
func handleUpload() http.HandlerFunc {
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
		if currentUserMuted(r) {
			apiErr(w, 403, "该账号已被限制发布，无法进行此操作")
			return
		}

		// multipart 大小限制（200MB）
		if err := r.ParseMultipartForm(200 * 1024 * 1024); err != nil {
			apiErr(w, 400, "文件过大或格式错误")
			return
		}
		defer r.MultipartForm.RemoveAll()

		courseCode := strings.TrimSpace(r.FormValue("course_code"))
		title := strings.TrimSpace(r.FormValue("title"))
		category := strings.TrimSpace(r.FormValue("category"))   // 历年试卷/课程作业/学习笔记
		teacher := strings.TrimSpace(r.FormValue("teacher"))
		semester := strings.TrimSpace(r.FormValue("semester"))
		description := strings.TrimSpace(r.FormValue("description"))
		anon := r.FormValue("anonymous") == "1"
		if courseCode == "" {
			apiErr(w, 400, "缺少课程标识")
			return
		}
		// 关键词自动审查拦截 (V8)：标题/描述/文件名
		// 注意：文件名由 multipart 提供，读文件后再取 hdr.Filename
		file, hdr, err := r.FormFile("file")
		if err != nil {
			apiErr(w, 400, "未收到文件")
			return
		}
		defer file.Close()
		// 敏感词 (F1)：命中 → 待复核，审核通过后公开
		fstatus := "正常"
		if hits := hitWords(title + " " + description + " " + hdr.Filename); len(hits) > 0 {
			fstatus = "待复核"
		}
		buf, err := io.ReadAll(file)
		if err != nil {
			apiErr(w, 400, "文件读取失败")
			return
		}
		if len(buf) == 0 {
			apiErr(w, 400, "文件为空")
			return
		}
		// 强制 200MB 上限（ParseMultipartForm 只控制内存缓冲，这里真正校验总大小）
		const maxUpload = 200 * 1024 * 1024
		if len(buf) > maxUpload {
			apiErr(w, 400, "文件超过 200MB 上限")
			return
		}
		// ---- V12 安全防护：危险类型 + 病毒扫描 ----
		if bad, reason := checkDangerousFile(hdr.Filename, buf); bad {
			apiErr(w, 400, "上传被安全拦截："+reason)
			return
		}
		if clean, msg := clamavScan(buf, hdr.Filename); !clean {
			apiErr(w, 400, "上传被安全拦截（病毒扫描）："+msg)
			return
		}
		//（≤200MB 已限制；ClamAV 通过环境变量 CLAMAV_CMD 启用）
		if err := saveFile(uid, courseCode, title, category, teacher, semester, description, hdr.Filename, buf, anon, fstatus); err != nil {
			apiErr(w, 500, "保存失败")
			return
		}
		if fstatus == "待复核" {
			apiJSON(w, 200, map[string]string{"message": "资料含敏感词，已进入待审队列，审核通过后展示"})
			return
		}
		apiJSON(w, 200, map[string]any{"message": "上传成功，等待审核", "size": len(buf)})
	}
}

// ---- 下载接口 ----
// handleFilesRouter 分发 /api/files/{id}/download 与 /api/files/{id}/preview
// (A1 修正) 下载/预览保持公开访问，无需登录
func handleFilesRouter() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/preview") {
			handleFilePreview()(w, r)
			return
		}
		handleFileDownload()(w, r)
	}
}

func handleFileDownload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// /api/files/{id}/download
		var id int64
		seg := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/files/"), "/")
		fmt.Sscanf(seg[0], "%d", &id)
		var stored, fname string
		err := authStore.db.QueryRow("SELECT stored_name, file_name FROM files WHERE id = ?", id).Scan(&stored, &fname)
		if err != nil {
			apiErr(w, 404, "文件不存在")
			return
		}
		path := filepath.Join("uploads", stored)
		if _, serr := os.Stat(path); os.IsNotExist(serr) {
			apiErr(w, 404, "文件已丢失")
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+fname+`"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, path)
	}
}

// handleFilePreview 以 inline 方式返回文件，供浏览器内嵌预览（图片/pdf/md等）
// /api/files/{id}/preview
func handleFilePreview() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seg := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/files/"), "/")
		var id int64
		fmt.Sscanf(seg[0], "%d", &id)
		var stored, fname string
		err := authStore.db.QueryRow("SELECT stored_name, file_name FROM files WHERE id = ?", id).Scan(&stored, &fname)
		if err != nil {
			apiErr(w, 404, "文件不存在")
			return
		}
		path := filepath.Join("uploads", stored)
		if _, serr := os.Stat(path); os.IsNotExist(serr) {
			apiErr(w, 404, "文件已丢失")
			return
		}
		// 设置正确的 Content-Type（inline 预览）
		ct := mimeByExt(fname)
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Disposition", "inline; filename=\""+fname+"\"")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, path)
	}
}

// mimeByExt 根据扩展名返回 Content-Type（供预览）
func mimeByExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".md", ".mdx":
		return "text/markdown; charset=utf-8"
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".txt", ".log":
		return "text/plain; charset=utf-8"
	case ".mp4", ".webm":
		return "video/mp4"
	case ".mp3", ".wav":
		return "audio/mpeg"
	case ".json":
		return "application/json; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// ---- 评价 ----
func handleListReviews() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			apiErr(w, 400, "缺少课程")
			return
		}
		rows, err := authStore.db.Query(
			`SELECT r.id, r.rating, r.content, r.is_anonymous, r.created_at, COALESCE(u.nickname,'匿名')
			 FROM reviews r LEFT JOIN users u ON u.id = r.user_id
			 WHERE r.course_code = ? AND r.status='正常' AND COALESCE(u.banned,0)=0 ORDER BY r.id DESC`, code)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		defer rows.Close()
		var out []map[string]any
		for rows.Next() {
			var id int64
			var rating int
			var content, created, nickname string
			var anonI int
			rows.Scan(&id, &rating, &content, &anonI, &created, &nickname)
			name := nickname
			if anonI == 1 || nickname == "" { name = "匿名" }
			out = append(out, map[string]any{
				"id": id, "rating": rating, "content": content,
				"author": name, "anonymous": anonI == 1, "createdAt": created,
			})
		}
		apiJSON(w, 200, map[string]any{"reviews": out})
	}
}

func handleAddReview() http.HandlerFunc {
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
		if currentUserMuted(r) {
			apiErr(w, 403, "该账号已被限制发布，无法进行此操作")
			return
		}
		// 邮箱验证（斐波那契触发）
		if requireEmailVerified(uid) {
			apiErr(w, 403, "请先验证邮箱后再继续发布（验证码见 /api/email/verify）")
			return
		}

		var body struct {
			CourseCode string `json:"courseCode"`
			Rating     int    `json:"rating"`
			Content    string `json:"content"`
			Anonymous  bool   `json:"anonymous"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		body.Content = strings.TrimSpace(body.Content)
		if body.CourseCode == "" || body.Content == "" {
			apiErr(w, 400, "请填写评价内容")
			return
		}
		// 关键词自动审查 (F1)：命中敏感词 → 待复核，审核通过后公开
		rstatus := "正常"
		if hits := hitWords(body.Content); len(hits) > 0 {
			rstatus = "待复核"
		}
		if body.Rating < 0 || body.Rating > 5 {
			body.Rating = 0
		}
		_, err := authStore.db.Exec(
			"INSERT INTO reviews (course_code, user_id, rating, content, is_anonymous, status, created_at) VALUES (?,?,?,?,?,?,?)",
			body.CourseCode, uid, body.Rating, body.Content, b2i(body.Anonymous), rstatus, time.Now().Format("2006-01-02 15:04:05"))
		if err != nil {
			apiErr(w, 500, "评价失败")
			return
		}
		if rstatus == "待复核" {
			apiJSON(w, 200, map[string]string{"message": "评价含敏感词，已进入待审队列，审核通过后展示"})
			return
		}
		apiJSON(w, 200, map[string]string{"message": "评价成功"})
	}
}

// ---- 工具 ----
func b2i(b bool) int {
	if b { return 1 }
	return 0
}



// ============ V6 后台资料管理 ============

// handleAdminListFiles 后台资料列表（含真实作者 email/nickname，即使匿名）
func handleAdminListFiles(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		page := atoi(r.URL.Query().Get("page"))
		if page < 1 { page = 1 }
		pageSize := atoi(r.URL.Query().Get("pageSize"))
		if pageSize < 1 || pageSize > 100 { pageSize = 30 }
		where := ""
		args := []any{}
		if q != "" {
			where = ` WHERE f.title LIKE ? OR f.file_name LIKE ? OR f.course_code LIKE ? OR u.nickname LIKE ? OR u.email LIKE ?`
			args = append(args, "%"+q+"%", "%"+q+"%", "%"+q+"%", "%"+q+"%", "%"+q+"%")
		}
		var total int
		auth.db.QueryRow(`SELECT COUNT(*) FROM files f LEFT JOIN users u ON u.id=f.uploader_id`+where, args...).Scan(&total)
		args = append(args, pageSize, (page-1)*pageSize)
		sqlq := `SELECT f.id, f.course_code, f.title, f.category, f.file_name, f.size,
			        f.uploader_id, f.is_anonymous, f.status, f.created_at,
			        COALESCE(u.nickname,''), COALESCE(u.email,'')
			 FROM files f LEFT JOIN users u ON u.id=f.uploader_id` + where + ` ORDER BY f.id DESC LIMIT ? OFFSET ?`
		rows, err := auth.db.Query(sqlq, args...)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		defer rows.Close()
		type item struct {
			ID         int64  `json:"id"`
			CourseCode string `json:"courseCode"`
			Title      string `json:"title"`
			Category   string `json:"category"`
			FileName   string `json:"fileName"`
			Size       int64  `json:"size"`
			UploaderID int64  `json:"uploaderId"`
			RealAuthor string `json:"realAuthor"`
			Anonymous  bool   `json:"anonymous"`
			Status     string `json:"status"`
			CreatedAt  string `json:"createdAt"`
		}
		out := []item{}
		for rows.Next() {
			it := item{}
			var anonI int
			var nick, email string
			rows.Scan(&it.ID, &it.CourseCode, &it.Title, &it.Category, &it.FileName,
				&it.Size, &it.UploaderID, &anonI, &it.Status, &it.CreatedAt, &nick, &email)
			it.Anonymous = anonI == 1
			it.RealAuthor = nick + " (" + email + ")"
			out = append(out, it)
		}
		apiJSON(w, 200, map[string]any{"total": total, "page": page, "pageSize": pageSize, "files": out})
	}
}

// handleAdminFileStatus 下架/恢复资料
// POST /api/admin/files/{id}/status  body: {"status":"正常"|"已下架"}
func handleAdminFileStatus(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/api/admin/files/")
		idStr := strings.TrimSuffix(p, "/status")
		id := parseID(idStr)
		if id == 0 {
			apiErr(w, 400, "无效的资料ID")
			return
		}
		var body struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		if body.Status != "正常" && body.Status != "已下架" {
			apiErr(w, 400, "状态无效")
			return
		}
		_, err := auth.db.Exec("UPDATE files SET status=? WHERE id=?", body.Status, id)
		if err != nil {
			apiErr(w, 500, "操作失败")
			return
		}
		apiJSON(w, 200, map[string]any{"ok": true, "id": id, "status": body.Status})
	}
}

// ============ V7 后台评价管理 ============

// handleAdminListReviews 后台评价列表（含真实作者，即使匿名）
// GET /api/admin/reviews
func handleAdminListReviews(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		page := atoi(r.URL.Query().Get("page"))
		if page < 1 { page = 1 }
		pageSize := atoi(r.URL.Query().Get("pageSize"))
		if pageSize < 1 || pageSize > 100 { pageSize = 30 }
		where := ""
		args := []any{}
		if q != "" {
			where = ` WHERE r.content LIKE ? OR r.course_code LIKE ? OR u.nickname LIKE ? OR u.email LIKE ?`
			args = append(args, "%"+q+"%", "%"+q+"%", "%"+q+"%", "%"+q+"%")
		}
		var total int
		auth.db.QueryRow(`SELECT COUNT(*) FROM reviews r LEFT JOIN users u ON u.id=r.user_id`+where, args...).Scan(&total)
		args = append(args, pageSize, (page-1)*pageSize)
		sqlq := `SELECT r.id, r.course_code, r.rating, r.content, r.is_anonymous, r.status, r.created_at,
			        r.user_id, COALESCE(u.nickname,''), COALESCE(u.email,'')
			 FROM reviews r LEFT JOIN users u ON u.id=r.user_id` + where + ` ORDER BY r.id DESC LIMIT ? OFFSET ?`
		rows, err := auth.db.Query(sqlq, args...)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		defer rows.Close()
		type item struct {
			ID         int64  `json:"id"`
			CourseCode string `json:"courseCode"`
			Rating     int    `json:"rating"`
			Content    string `json:"content"`
			Anonymous  bool   `json:"anonymous"`
			Status     string `json:"status"`
			CreatedAt  string `json:"createdAt"`
			UserID     int64  `json:"userId"`
			RealAuthor string `json:"realAuthor"`
		}
		out := []item{}
		for rows.Next() {
			it := item{}
			var anonI int
			var nick, email string
			rows.Scan(&it.ID, &it.CourseCode, &it.Rating, &it.Content, &anonI, &it.Status, &it.CreatedAt,
				&it.UserID, &nick, &email)
			it.Anonymous = anonI == 1
			it.RealAuthor = nick + " (" + email + ")"
			out = append(out, it)
		}
		apiJSON(w, 200, map[string]any{"total": total, "page": page, "pageSize": pageSize, "reviews": out})
	}
}

// handleAdminReviewStatus 下架/恢复评价
// POST /api/admin/reviews/{id}/status  body: {"status":"正常"|"已下架"}
func handleAdminReviewStatus(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(auth, w, r) == 0 {
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/api/admin/reviews/")
		idStr := strings.TrimSuffix(p, "/status")
		id := parseID(idStr)
		if id == 0 {
			apiErr(w, 400, "无效的评价ID")
			return
		}
		var body struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		if body.Status != "正常" && body.Status != "已下架" {
			apiErr(w, 400, "状态无效")
			return
		}
		_, err := auth.db.Exec("UPDATE reviews SET status=? WHERE id=?", body.Status, id)
		if err != nil {
			apiErr(w, 500, "操作失败")
			return
		}
		apiJSON(w, 200, map[string]any{"ok": true, "id": id, "status": body.Status})
	}
}