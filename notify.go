// notify.go — V13 (D1) 通知中心
// 触发：帖子被点赞、被评论 → 通知作者
// 接口：
//   GET  /api/notifications    我的通知（最新在前，含未读数）
//   POST /api/notifications/read   全部标记已读
package main

import (
	"net/http"
	"time"
)

// addNotification 写一条通知（不通知自己）
func addNotification(recipientID, actorID int64, typ, text string, refID int64) {
	if recipientID <= 0 || recipientID == actorID {
		return
	}
	_, err := authStore.db.Exec(
		"INSERT INTO notifications (recipient_id, actor_id, type, ref_id, text, is_read, created_at) VALUES (?,?,?,?,?,0,?)",
		recipientID, actorID, typ, refID, text, time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		return
	}
}

// handleMyNotifications 我的通知
func handleMyNotifications() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 {
			apiErr(w, 401, "请先登录")
			return
		}
		rows, err := authStore.db.Query(
			`SELECT n.id, n.type, n.text, n.is_read, n.ref_id, n.created_at,
			        COALESCE(u.nickname,'')
			 FROM notifications n LEFT JOIN users u ON u.id=n.actor_id
			 WHERE n.recipient_id=? AND (n.actor_id<=0 OR n.actor_id NOT IN (SELECT blocked_id FROM blocklist WHERE blocker_id=?))
			 ORDER BY n.id DESC LIMIT 100`, uid, uid)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		defer rows.Close()
		type item struct {
			ID        int64  `json:"id"`
			Type      string `json:"type"`
			Text      string `json:"text"`
			IsRead    bool   `json:"isRead"`
			RefID     int64  `json:"refId"`
			CreatedAt string `json:"createdAt"`
			Actor     string `json:"actor"`
		}
		var out []item
		var unread int
		for rows.Next() {
			it := item{}
			var rd int
			var nick string
			rows.Scan(&it.ID, &it.Type, &it.Text, &rd, &it.RefID, &it.CreatedAt, &nick)
			it.IsRead = rd == 1
			it.Actor = nick
			out = append(out, it)
			if rd == 0 {
				unread++
			}
		}
		apiJSON(w, 200, map[string]any{"notifications": out, "unread": unread})
	}
}

// handleReadNotifications 标记全部已读
func handleReadNotifications() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 {
			apiErr(w, 401, "请先登录")
			return
		}
		_, _ = authStore.db.Exec("UPDATE notifications SET is_read=1 WHERE recipient_id=?", uid)
		apiJSON(w, 200, map[string]any{"ok": true})
	}
}

// notifyPostAuthor 点赞/评论通用：通知帖子作者
func notifyPostAuthor(postID int64, actorID int64, typ, text string) {
	var owner int64
	authStore.db.QueryRow("SELECT user_id FROM posts WHERE id=?", postID).Scan(&owner)
	addNotification(owner, actorID, typ, text, postID)
}