// favorites.go — J6 收藏 + 相册
// 用户可收藏内容（帖子/文章/资料/课程），并维护相册（把收藏/内容分组归档，可设私密）。
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// migrateFavorites 建表：favorites（收藏） + albums + album_items
func migrateFavorites(db *sql.DB) {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS favorites (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id     INTEGER NOT NULL,
		target_type TEXT NOT NULL DEFAULT '',
		target_id   TEXT NOT NULL DEFAULT '',
		created_at  TEXT NOT NULL
	)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS albums (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id     INTEGER NOT NULL,
		title       TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		cover       TEXT NOT NULL DEFAULT '',
		is_private  INTEGER NOT NULL DEFAULT 0,
		created_at  TEXT NOT NULL
	)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS album_items (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		album_id   INTEGER NOT NULL,
		user_id    INTEGER NOT NULL,
		target_type TEXT NOT NULL DEFAULT '',
		target_id  TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`)
}

// ---- 收藏 ----

// handleToggleFavorite 收藏/取消收藏
// POST /api/favorite/toggle  body:{targetType, targetId}
func handleToggleFavorite() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 { apiErr(w, 401, "请先登录"); return }
		var body struct {
			TargetType string `json:"targetType"`
			TargetID   string `json:"targetId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误"); return
		}
		body.TargetType = strings.TrimSpace(body.TargetType)
		body.TargetID = strings.TrimSpace(body.TargetID)
		if body.TargetType == "" || body.TargetID == "" {
			apiErr(w, 400, "缺少收藏对象"); return
		}
		var cnt int
		authStore.db.QueryRow(`SELECT COUNT(*) FROM favorites WHERE user_id=? AND target_type=? AND target_id=?`,
			uid, body.TargetType, body.TargetID).Scan(&cnt)
		if cnt > 0 {
			_, _ = authStore.db.Exec(`DELETE FROM favorites WHERE user_id=? AND target_type=? AND target_id=?`,
				uid, body.TargetType, body.TargetID)
			apiJSON(w, 200, map[string]any{"favorited": false})
			return
		}
		_, _ = authStore.db.Exec(`INSERT INTO favorites (user_id,target_type,target_id,created_at) VALUES (?,?,?,?)`,
			uid, body.TargetType, body.TargetID, time.Now().Format("2006-01-02 15:04:05"))
		apiJSON(w, 200, map[string]any{"favorited": true})
	}
}

// isFavorited 判断某用户是否已收藏
func isFavorited(uid int64, targetType, targetID string) bool {
	if uid == 0 { return false }
	var cnt int
	authStore.db.QueryRow(`SELECT COUNT(*) FROM favorites WHERE user_id=? AND target_type=? AND target_id=?`,
		uid, targetType, targetID).Scan(&cnt)
	return cnt > 0
}

// handleMyFavorites 我的收藏列表
// GET /api/favorites?type=post|article|file|course
func handleMyFavorites() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 { apiErr(w, 401, "请先登录"); return }
		t := strings.TrimSpace(r.URL.Query().Get("type"))
		rows, err := authStore.db.Query(`SELECT id, target_type, target_id, created_at FROM favorites WHERE user_id=? ORDER BY id DESC`, uid)
		if err != nil {
			apiErr(w, 500, "查询失败"); return
		}
		defer rows.Close()
		type item struct {
			ID         int64  `json:"id"`
			TargetType string `json:"targetType"`
			TargetID   string `json:"targetId"`
			CreatedAt  string `json:"createdAt"`
		}
		var out []item
		for rows.Next() {
			it := item{}
			rows.Scan(&it.ID, &it.TargetType, &it.TargetID, &it.CreatedAt)
			if t != "" && it.TargetType != t { continue }
			out = append(out, it)
		}
		apiJSON(w, 200, map[string]any{"favorites": out, "total": len(out)})
	}
}

// ---- 相册 ----

// handleAlbums 相册列表（owner/自己看全部，别人只看公开）
// GET /api/albums?user_id=&mine=1
func handleAlbums() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		ownerID := int64(0)
		if oid := r.URL.Query().Get("user_id"); oid != "" {
			ownerID, _ = strconv.ParseInt(oid, 10, 64)
		}
		if r.URL.Query().Get("mine") == "1" {
			ownerID = uid
		}
		var rows *sql.Rows
		var err error
		if ownerID > 0 && ownerID == uid {
			rows, err = authStore.db.Query(`SELECT id,user_id,title,description,cover,is_private,created_at FROM albums WHERE user_id=? ORDER BY id DESC`, ownerID)
		} else if ownerID > 0 {
			rows, err = authStore.db.Query(`SELECT id,user_id,title,description,cover,is_private,created_at FROM albums WHERE user_id=? AND is_private=0 ORDER BY id DESC`, ownerID)
		} else {
			rows, err = authStore.db.Query(`SELECT id,user_id,title,description,cover,is_private,created_at FROM albums WHERE is_private=0 ORDER BY id DESC`)
		}
		if err != nil { apiErr(w, 500, "查询失败"); return }
		defer rows.Close()
		type album struct {
			ID          int64  `json:"id"`
			UserID      int64  `json:"userId"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Cover       string `json:"cover"`
			IsPrivate   int    `json:"isPrivate"`
			CreatedAt   string `json:"createdAt"`
		}
		var out []album
		for rows.Next() {
			a := album{}
			rows.Scan(&a.ID, &a.UserID, &a.Title, &a.Description, &a.Cover, &a.IsPrivate, &a.CreatedAt)
			out = append(out, a)
		}
		apiJSON(w, 200, map[string]any{"albums": out, "total": len(out)})
	}
}

// handleAlbumCreate 新建相册
// POST /api/albums  body:{title,description,isPrivate,cover}
func handleAlbumCreate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 { apiErr(w, 401, "请先登录"); return }
		var body struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			IsPrivate   int    `json:"isPrivate"`
			Cover       string `json:"cover"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误"); return
		}
		body.Title = strings.TrimSpace(body.Title)
		body.Description = strings.TrimSpace(body.Description)
		if body.Title == "" {
			apiErr(w, 400, "请填写相册名称"); return
		}
		body.Title = cleanUserText(body.Title, 40)
		body.Description = cleanUserText(body.Description, 100)
		body.Cover = cleanUserText(body.Cover, 200)
		res, err := authStore.db.Exec(`INSERT INTO albums (user_id,title,description,cover,is_private,created_at) VALUES (?,?,?,?,?,?)`,
			uid, body.Title, body.Description, body.Cover, body.IsPrivate, time.Now().Format("2006-01-02 15:04:05"))
		if err != nil { apiErr(w, 500, "创建失败"); return }
		id, _ := res.LastInsertId()
		apiJSON(w, 200, map[string]any{"ok": true, "id": id})
	}
}

// handleAlbumDelete 删除相册
// POST /api/albums/delete  body:{id}
func handleAlbumDelete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 { apiErr(w, 401, "请先登录"); return }
		var body struct{ ID int64 `json:"id"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == 0 {
			apiErr(w, 400, "请求格式错误"); return
		}
		var owner int64
		authStore.db.QueryRow(`SELECT user_id FROM albums WHERE id=?`, body.ID).Scan(&owner)
		if owner == 0 || owner != uid {
			apiErr(w, 403, "无权操作"); return
		}
		_, _ = authStore.db.Exec(`DELETE FROM albums WHERE id=?`, body.ID)
		_, _ = authStore.db.Exec(`DELETE FROM album_items WHERE album_id=?`, body.ID)
		apiJSON(w, 200, map[string]any{"ok": true})
	}
}

// handleAlbumAddItem 把内容加入相册
// POST /api/albums/add-item  body:{albumId,targetType,targetId}
func handleAlbumAddItem() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 { apiErr(w, 401, "请先登录"); return }
		var body struct {
			AlbumID    int64  `json:"albumId"`
			TargetType string `json:"targetType"`
			TargetID   string `json:"targetId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误"); return
		}
		var owner int64
		authStore.db.QueryRow(`SELECT user_id FROM albums WHERE id=?`, body.AlbumID).Scan(&owner)
		if owner == 0 || owner != uid {
			apiErr(w, 403, "无权操作"); return
		}
		var cnt int
		authStore.db.QueryRow(`SELECT COUNT(*) FROM album_items WHERE album_id=? AND target_type=? AND target_id=?`,
			body.AlbumID, body.TargetType, body.TargetID).Scan(&cnt)
		if cnt > 0 {
			apiErr(w, 400, "该内容已在相册中"); return
		}
		_, _ = authStore.db.Exec(`INSERT INTO album_items (album_id,user_id,target_type,target_id,created_at) VALUES (?,?,?,?,?)`,
			body.AlbumID, uid, body.TargetType, body.TargetID, time.Now().Format("2006-01-02 15:04:05"))
		apiJSON(w, 200, map[string]any{"ok": true})
	}
}

// handleAlbumItems 相册内容
// GET /api/albums/items?album_id=
func handleAlbumItems() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		albumID, _ := strconv.ParseInt(r.URL.Query().Get("album_id"), 10, 64)
		if albumID == 0 { apiErr(w, 400, "缺少相册ID"); return }
		var isPrivate int
		var owner int64
		authStore.db.QueryRow(`SELECT is_private,user_id FROM albums WHERE id=?`, albumID).Scan(&isPrivate, &owner)
		if owner == 0 {
			apiErr(w, 404, "相册不存在"); return
		}
		uid := currentUserID(r)
		if isPrivate == 1 && uid != owner {
			apiErr(w, 403, "该相册未公开"); return
		}
		rows, err := authStore.db.Query(`SELECT id,album_id,target_type,target_id,created_at FROM album_items WHERE album_id=? ORDER BY id DESC`, albumID)
		if err != nil { apiErr(w, 500, "查询失败"); return }
		defer rows.Close()
		type item struct {
			ID         int64  `json:"id"`
			AlbumID    int64  `json:"albumId"`
			TargetType string `json:"targetType"`
			TargetID   string `json:"targetId"`
			CreatedAt  string `json:"createdAt"`
		}
		var out []item
		for rows.Next() {
			it := item{}
			rows.Scan(&it.ID, &it.AlbumID, &it.TargetType, &it.TargetID, &it.CreatedAt)
			out = append(out, it)
		}
		apiJSON(w, 200, map[string]any{"items": out, "albumId": albumID})
	}
}
