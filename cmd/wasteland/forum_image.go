// forum_image.go — 论坛/文章图片上传（V16.2）
// 发帖/评论/文章正文中的图片，上传到 uploads/forum-images/，返回可访问 URL。
// 前端把 URL 以 ![图片](url) 形式插入 Markdown 正文。
// POST /api/upload-image  multipart: file=<image>
// GET  /forum-image-file/{name}  静态访问
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// handleUploadImage 上传正文图片（登录用户，≤8MB，图片类型）
func handleUploadImage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 { apiErr(w, 401, "请先登录"); return }
		if currentUserMuted(r) { apiErr(w, 403, "该账号已被限制发布"); return }
		if err := r.ParseMultipartForm(8 * 1024 * 1024); err != nil {
			apiErr(w, 400, "图片过大（限 8MB）或格式错误"); return
		}
		defer r.MultipartForm.RemoveAll()
		file, hdr, err := r.FormFile("file")
		if err != nil { apiErr(w, 400, "未收到文件"); return }
		defer file.Close()
		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		default:
			apiErr(w, 400, "仅支持 jpg/png/gif/webp 图片"); return
		}
		data, err := io.ReadAll(io.LimitReader(file, 8*1024*1024))
		if err != nil || len(data) == 0 { apiErr(w, 400, "文件为空"); return }
		if len(data) > 8*1024*1024 { apiErr(w, 400, "图片超过 8MB 上限"); return }
		// 魔数嗅探
		head := data
		if len(head) > 12 { head = head[:12] }
		isImg := strings.HasPrefix(string(head), "\xff\xd8") ||
			strings.HasPrefix(string(head), "\x89PNG") ||
			strings.HasPrefix(string(head), "GIF8") ||
			(strings.HasPrefix(string(head), "RIFF") && len(head) > 8 && string(head[8:12]) == "WEBP")
		if !isImg { apiErr(w, 400, "文件内容不是有效图片"); return }
		dir := filepath.Join("uploads", "forum-images")
		if err := os.MkdirAll(dir, 0o755); err != nil { apiErr(w, 500, "存储失败"); return }
		stored := fmt.Sprintf("img_%d_%d%s", time.Now().UnixNano(), uid, ext)
		path := filepath.Join(dir, stored)
		if err := os.WriteFile(path, data, 0o644); err != nil { apiErr(w, 500, "保存失败"); return }
		url := "/forum-image-file/" + stored
		apiJSON(w, 200, map[string]any{"url": url, "size": len(data)})
	}
}

// handleForumImageFile 提供正文图片静态文件
func handleForumImageFile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/forum-image-file/")
		if strings.Contains(name, "..") || strings.Contains(name, "/") || name == "" {
			http.NotFound(w, r); return
		}
		path := filepath.Join("uploads", "forum-images", name)
		if _, err := os.Stat(path); os.IsNotExist(err) { http.NotFound(w, r); return }
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, path)
	}
}
