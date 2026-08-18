// message.go — V13 (D2) 私信
// 规则：仅双方「互动过」才可互相私信。互动 = 一方曾在另一方的帖子/文章下评论/发帖被评论。
// 接口：
//   GET  /api/message/eligible?to=xx  是否可互发私信（返回 eligible + 双方昵称）
//   GET  /api/messages               我的收件箱（含未读数）
//   POST /api/messages               发私信 body:{to,content}
//   POST /api/messages/read          收件箱全部已读
package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// interacted 判定 a 与 b 是否互动过（a 在 b 的内容下活动 或 b 在 a 的内容下活动）
func interacted(a, b int64) bool {
	if a <= 0 || b <= 0 {
		return false
	}
	// a 评论过 b 的帖子
	var n1, n2, n3 int
	// b 的帖子被 a 评论过
	authStore.db.QueryRow(
		`SELECT COUNT(*) FROM comments c JOIN posts p ON p.id=c.post_id WHERE p.user_id=? AND c.user_id=?`, b, a).Scan(&n1)
	// a 的帖子被 b 评论过
	authStore.db.QueryRow(
		`SELECT COUNT(*) FROM comments c JOIN posts p ON p.id=c.post_id WHERE p.user_id=? AND c.user_id=?`, a, b).Scan(&n2)
	// a 发过回复 b？文章中暂不计，先按评论
	n3 = 0
	return n1+n2+n3 > 0
}

// handleMessageEligible 私信资格查询
func handleMessageEligible() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := currentUserID(r)
		if me == 0 {
			apiErr(w, 401, "请先登录")
			return
		}
		to := parseID(r.URL.Query().Get("to"))
		if to == 0 {
			apiErr(w, 400, "无效用户")
			return
		}
		// 对方昵称
		var name string
		authStore.db.QueryRow("SELECT COALESCE(nickname,'') FROM users WHERE id=?", to).Scan(&name)
		apiJSON(w, 200, map[string]any{
			"eligible": interacted(me, to),
			"targetName": name,
		})
	}
}

// handleMyMessages 我的收件箱
func handleMyMessages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := currentUserID(r)
		if me == 0 {
			apiErr(w, 401, "请先登录")
			return
		}
		rows, err := authStore.db.Query(
			`SELECT m.id, m.from_id, m.content, m.is_read, m.created_at, COALESCE(u.nickname,'')
			 FROM messages m LEFT JOIN users u ON u.id=m.from_id
			 WHERE m.to_id=? AND m.from_id NOT IN (SELECT blocked_id FROM blocklist WHERE blocker_id=?)
			 ORDER BY m.id DESC LIMIT 100`, me, me)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		defer rows.Close()
		type item struct {
			ID        int64  `json:"id"`
			FromID    int64  `json:"fromId"`
			FromName  string `json:"fromName"`
			Content   string `json:"content"`
			IsRead    bool   `json:"isRead"`
			CreatedAt string `json:"createdAt"`
		}
		var out []item
		var unread int
		for rows.Next() {
			it := item{}
			var rd int
			var nick string
			rows.Scan(&it.ID, &it.FromID, &it.Content, &rd, &it.CreatedAt, &nick)
			it.IsRead = rd == 1
			it.FromName = nick
			out = append(out, it)
			if rd == 0 {
				unread++
			}
		}
		apiJSON(w, 200, map[string]any{"messages": out, "unread": unread})
	}
}

// handleSendMessage 发私信（校验互动）
func handleSendMessage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := currentUserID(r)
		if me == 0 {
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

		var body struct {
			To      int64  `json:"to"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		// G5：若对方把我拉了黑，则无法再发私信
		if isBlocked(authStore.db, body.To, me) {
			apiErr(w, 403, "对方已将你拉黑，无法发送私信")
			return
		}
		body.Content = strings.TrimSpace(body.Content)
		if body.To == 0 || body.To == me {
			apiErr(w, 400, "私信对象无效")
			return
		}
		if body.Content == "" {
			apiErr(w, 400, "私信内容不能为空")
			return
		}
		if len([]rune(body.Content)) > 2000 {
			apiErr(w, 400, "私信内容过长")
			return
		}
		if !interacted(me, body.To) {
			apiErr(w, 403, "你们还没有互动过，暂不能私信")
			return
		}
		if hits := hitWords(body.Content); len(hits) > 0 {
			apiErr(w, 400, "内容包含敏感词: "+strings.Join(hits, "、"))
			return
		}
		_, err := authStore.db.Exec(
			"INSERT INTO messages (from_id, to_id, content, is_read, created_at) VALUES (?,?,?,0,?)",
			me, body.To, body.Content, time.Now().Format("2006-01-02 15:04:05"))
		if err != nil {
			apiErr(w, 500, "发送失败")
			return
		}
		// 同时写一条通知给收件人
		addNotification(body.To, me, "message", "私信了你", 0)
		apiJSON(w, 200, map[string]string{"message": "私信已发送"})
	}
}

// handleReadMessages 收件箱全部已读
func handleReadMessages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := currentUserID(r)
		if me == 0 {
			apiErr(w, 401, "请先登录")
			return
		}
		_, _ = authStore.db.Exec("UPDATE messages SET is_read=1 WHERE to_id=?", me)
		apiJSON(w, 200, map[string]any{"ok": true})
	}
}