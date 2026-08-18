// album_photo.go — 相册照片上传与访问
// 上传路径：uploads/albums/ 下随机文件名；album_items 记 target_type='photo'，target_id=文件名
// POST /api/albums/upload-photo  multipart: file=<image>, albumId=<id>
// GET  /album-photo-file/{name}  静态访问照片
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// handleAlbumUploadPhoto 上传照片到相册
func handleAlbumUploadPhoto() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 { apiErr(w, 401, "请先登录"); return }
		if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
			apiErr(w, 400, "文件过大（照片限 10MB）或格式错误"); return
		}
		defer r.MultipartForm.RemoveAll()
		albumID, _ := strconv.ParseInt(r.FormValue("albumId"), 10, 64)
		if albumID == 0 { apiErr(w, 400, "缺少相册ID"); return }
		var owner int64
		authStore.db.QueryRow(`SELECT user_id FROM albums WHERE id=?`, albumID).Scan(&owner)
		if owner == 0 || owner != uid { apiErr(w, 403, "无权操作"); return }

		file, hdr, err := r.FormFile("file")
		if err != nil { apiErr(w, 400, "未收到文件"); return }
		defer file.Close()
		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		default:
			apiErr(w, 400, "仅支持 jpg/png/gif/webp 图片"); return
		}
		data, err := io.ReadAll(io.LimitReader(file, 10*1024*1024))
		if err != nil || len(data) == 0 { apiErr(w, 400, "文件为空"); return }
		if len(data) > 10*1024*1024 { apiErr(w, 400, "照片超过 10MB 上限"); return }
		// 魔数嗅探
		head := data
		if len(head) > 12 { head = head[:12] }
		isImg := strings.HasPrefix(string(head), "\xff\xd8") ||
			strings.HasPrefix(string(head), "\x89PNG") ||
			strings.HasPrefix(string(head), "GIF8") ||
			(strings.HasPrefix(string(head), "RIFF") && len(head) > 8 && string(head[8:12]) == "WEBP")
		if !isImg { apiErr(w, 400, "文件内容不是有效图片"); return }

		dir := filepath.Join("uploads", "albums")
		if err := os.MkdirAll(dir, 0o755); err != nil { apiErr(w, 500, "存储失败"); return }
		stored := fmt.Sprintf("al_%d_%d%s", time.Now().UnixNano(), uid, ext)
		path := filepath.Join(dir, stored)
		if err := os.WriteFile(path, data, 0o644); err != nil { apiErr(w, 500, "保存失败"); return }

		// 入相册项
		_, err = authStore.db.Exec(`INSERT INTO album_items (album_id,user_id,target_type,target_id,created_at) VALUES (?,?,?,?,?)`,
			albumID, uid, "photo", stored, time.Now().Format("2006-01-02 15:04:05"))
		if err != nil { os.Remove(path); apiErr(w, 500, "入册失败"); return }
		// 若无封面，用首张照片做封面
		var cov string
		authStore.db.QueryRow(`SELECT cover FROM albums WHERE id=?`, albumID).Scan(&cov)
		if cov == "" {
			_, _ = authStore.db.Exec(`UPDATE albums SET cover=? WHERE id=?`, "/album-photo-file/"+stored, albumID)
		}
		apiJSON(w, 200, map[string]any{"ok": true, "photo": "/album-photo-file/" + stored})
	}
}

// handleAlbumPhotoFile 提供相册照片静态文件
func handleAlbumPhotoFile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/album-photo-file/")
		if strings.Contains(name, "..") || strings.Contains(name, "/") || name == "" {
			http.NotFound(w, r); return
		}
		path := filepath.Join("uploads", "albums", name)
		if _, err := os.Stat(path); os.IsNotExist(err) { http.NotFound(w, r); return }
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, path)
	}
}


// handleAlbumSaveImage 保存外部图片到我的相册
// POST /api/albums/save-image  body:{url, albumId?}
// url 可以是站内图片（/forum-image-file/xx 或 /album-photo-file/xx）或站外 https 图片
func handleAlbumSaveImage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 { apiErr(w, 401, "请先登录"); return }
		var body struct {
			URL     string `json:"url"`
			AlbumID int64  `json:"albumId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误"); return
		}
		body.URL = strings.TrimSpace(body.URL)
		if body.URL == "" { apiErr(w, 400, "缺少图片地址"); return }
		// 只允许 http(s) 或站内相对路径
		if !strings.HasPrefix(body.URL, "/") && !strings.HasPrefix(body.URL, "http://") && !strings.HasPrefix(body.URL, "https://") {
			apiErr(w, 400, "图片地址不合法"); return
		}
		// 若指定了相册，校验归属
		if body.AlbumID != 0 {
			var owner int64
			authStore.db.QueryRow(`SELECT user_id FROM albums WHERE id=?`, body.AlbumID).Scan(&owner)
			if owner == 0 || owner != uid { apiErr(w, 403, "无权操作"); return }
		}
		// 拉取图片（站内路径转完整地址；站外直接下载）
		fullURL := body.URL
		if strings.HasPrefix(body.URL, "/") {
			fullURL = "http://" + r.Host + body.URL
		}
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(fullURL)
		if err != nil {
			apiErr(w, 502, "图片获取失败"); return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 { apiErr(w, 502, "图片获取失败（状态 "+fmt.Sprint(resp.StatusCode)+"）"); return }
		// 类型判断
		ct := resp.Header.Get("Content-Type")
		var ext string
		switch {
		case strings.Contains(ct, "jpeg") || strings.Contains(ct, "jpg"): ext = ".jpg"
		case strings.Contains(ct, "png"): ext = ".png"
		case strings.Contains(ct, "gif"): ext = ".gif"
		case strings.Contains(ct, "webp"): ext = ".webp"
		default: ext = ".jpg"
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
		if err != nil || len(data) == 0 { apiErr(w, 400, "图片为空"); return }
		if len(data) > 8*1024*1024 { apiErr(w, 400, "图片超过 8MB"); return }
		// 魔数嗅探
		head := data
		if len(head) > 12 { head = head[:12] }
		isImg := strings.HasPrefix(string(head), "\xff\xd8") ||
			strings.HasPrefix(string(head), "\x89PNG") ||
			strings.HasPrefix(string(head), "GIF8") ||
			(strings.HasPrefix(string(head), "RIFF") && len(head) > 8 && string(head[8:12]) == "WEBP")
		if !isImg { apiErr(w, 400, "内容不是有效图片"); return }
		// 存盘
		dir := filepath.Join("uploads", "albums")
		if err := os.MkdirAll(dir, 0o755); err != nil { apiErr(w, 500, "存储失败"); return }
		stored := fmt.Sprintf("al_%d_%d%s", time.Now().UnixNano(), uid, ext)
		path := filepath.Join(dir, stored)
		if err := os.WriteFile(path, data, 0o644); err != nil { apiErr(w, 500, "保存失败"); return }
		// 入相册（无相册则自动建"保存的图片"）
		albumID := body.AlbumID
		if albumID == 0 {
			var aid int64
			authStore.db.QueryRow(`SELECT id FROM albums WHERE user_id=? AND title='保存的图片'`, uid).Scan(&aid)
			if aid == 0 {
				res2, _ := authStore.db.Exec(`INSERT INTO albums (user_id,title,description,cover,is_private,created_at) VALUES (?,?,?,?,?,?)`,
					uid, "保存的图片", "从文章/论坛保存的图片", "", 0, time.Now().Format("2006-01-02 15:04:05"))
				aid, _ = res2.LastInsertId()
			}
			albumID = aid
		}
		_, err = authStore.db.Exec(`INSERT INTO album_items (album_id,user_id,target_type,target_id,created_at) VALUES (?,?,?,?,?)`,
			albumID, uid, "photo", stored, time.Now().Format("2006-01-02 15:04:05"))
		if err != nil { os.Remove(path); apiErr(w, 500, "入册失败"); return }
		// 相册无封面则用它
		var cov string
		authStore.db.QueryRow(`SELECT cover FROM albums WHERE id=?`, albumID).Scan(&cov)
		if cov == "" {
			_, _ = authStore.db.Exec(`UPDATE albums SET cover=? WHERE id=?`, "/album-photo-file/"+stored, albumID)
		}
		apiJSON(w, 200, map[string]any{"ok": true, "photo": "/album-photo-file/" + stored, "albumId": albumID})
	}
}