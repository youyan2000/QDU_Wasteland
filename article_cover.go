// article_cover.go — J7 文章封面上传/编辑
// 文章封面：支持作者上传图片（存 uploads/covers，以 /api/cover-file/ 提供访问），
// 也可不设（前端回退到分类色块）。cover 存 URL 字符串到 articles.cover。
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// migrateArticleCover 确保 articles 表含 cover 列 (J7)
func migrateArticleCover(db *sql.DB) error {
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('articles') WHERE name='cover'`).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		_, err := db.Exec(`ALTER TABLE articles ADD COLUMN cover TEXT NOT NULL DEFAULT ''`)
		return err
	}
	return nil
}

// allowedCoverExts 允许的封面图片扩展名
var allowedCoverExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}

// handleArticleCoverUpload 封面上传（登录）
// POST /api/article-cover   multipart/form-data: field "file"
// 返回 { cover:"/api/cover-file/xxx.jpg" }
func handleArticleCoverUpload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 { apiErr(w, 401, "请先登录"); return }
		r.Body = http.MaxBytesReader(w, r.Body, 5<<20) // 5MB
		if err := r.ParseMultipartForm(5 << 20); err != nil {
			apiErr(w, 400, "上传失败（图片过大或格式错误）"); return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			apiErr(w, 400, "请选择图片文件"); return
		}
		defer file.Close()
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if !allowedCoverExts[ext] {
			apiErr(w, 400, "仅支持 jpg/png/webp/gif 图片"); return
		}
		data, err := io.ReadAll(file)
		if err != nil || len(data) == 0 {
			apiErr(w, 400, "读取图片失败"); return
		}
		if err := os.MkdirAll(filepath.Join("uploads", "covers"), 0o755); err != nil {
			apiErr(w, 500, "存储目录创建失败"); return
		}
		name := fmt.Sprintf("cover_%d_%d%s", uid, time.Now().UnixNano(), ext)
		path := filepath.Join("uploads", "covers", name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			apiErr(w, 500, "保存图片失败"); return
		}
		apiJSON(w, 200, map[string]any{"cover": "/api/cover-file/" + name})
	}
}

// handleCoverFile 提供封面图片访问
// GET /api/cover-file/{name}
func handleCoverFile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/api/cover-file/")
		name = filepath.Base(name) // 防目录穿越
		if name == "" || name == "." {
			http.NotFound(w, r); return
		}
		path := filepath.Join("uploads", "covers", name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			http.NotFound(w, r); return
		}
		w.Header().Set("Content-Type", mimeByExt(name))
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.ServeFile(w, r, path)
	}
}

// ensureCoverField 兼容旧 JSON：可在 create/update 中带 cover 字段
var _ = json.Marshal
