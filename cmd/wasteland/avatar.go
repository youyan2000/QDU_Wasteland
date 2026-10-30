// avatar.go — 用户头像上传（V15）
// 上传路径：uploads/avatars/ 下，随机文件名；DB users.avatar 存访问 URL "/avatar-file/{name}"
// POST /api/avatar/upload   — 上传（multipart, 字段 "file"）
// GET  /avatar-file/{name}  — 静态访问头像文件
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

// handleAvatarUpload 上传当前用户头像
// POST /api/avatar/upload  multipart: file=<image>
func handleAvatarUpload() http.HandlerFunc {
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
		// 头像大小限制 2MB
		if err := r.ParseMultipartForm(2 * 1024 * 1024); err != nil {
			apiErr(w, 400, "文件过大（头像限 2MB）或格式错误")
			return
		}
		defer r.MultipartForm.RemoveAll()
		file, hdr, err := r.FormFile("file")
		if err != nil {
			apiErr(w, 400, "未收到文件")
			return
		}
		defer file.Close()
		// 只允许图片扩展名
		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		default:
			apiErr(w, 400, "头像仅支持 jpg/png/gif/webp")
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, 2*1024*1024))
		if err != nil || len(data) == 0 {
			apiErr(w, 400, "文件为空")
			return
		}
		if len(data) > 2*1024*1024 {
			apiErr(w, 400, "头像超过 2MB 上限")
			return
		}
		// 魔数嗅探：必须是真实图片（防伪装）
		head := data
		if len(head) > 12 { head = head[:12] }
		isImg := strings.HasPrefix(string(head), "\xff\xd8") || // jpg
			strings.HasPrefix(string(head), "\x89PNG") || // png
			strings.HasPrefix(string(head), "GIF8") || // gif
			(strings.HasPrefix(string(head), "RIFF") && len(head) > 8 && string(head[8:12]) == "WEBP") // webp
		if !isImg {
			apiErr(w, 400, "文件内容不是有效图片")
			return
		}
		// 存盘
		dir := filepath.Join("uploads", "avatars")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			apiErr(w, 500, "存储失败")
			return
		}
		stored := fmt.Sprintf("av_%d_%d%s", time.Now().UnixNano(), uid, ext)
		path := filepath.Join(dir, stored)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			apiErr(w, 500, "保存失败")
			return
		}
		url := "/avatar-file/" + stored
		// 更新数据库
		if _, err := authStore.db.Exec("UPDATE users SET avatar=? WHERE id=?", url, uid); err != nil {
			os.Remove(path)
			apiErr(w, 500, "更新失败")
			return
		}
		apiJSON(w, 200, map[string]string{"avatar": url})
	}
}

// handleAvatarFile 提供头像静态文件
func handleAvatarFile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/avatar-file/")
		if strings.Contains(name, "..") || strings.Contains(name, "/") || name == "" {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join("uploads", "avatars", name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		// 图片类可缓存
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, path)
	}
}
