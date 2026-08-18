// report.go — V8 前台举报提交
// 用户可举报：帖子(post / 帖子ID)、评论(comment / 评论ID)、课程评价(review / 评价ID)、资料(file / 文件ID)、文章(article / 文章ID)
// 举报写入 reports 表（status='待处理'），管理员在后台处理（下架目标/忽略）。
package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// handleReport 提交举报
// POST /api/report  body: {"targetType":"post|comment|review|file|article","targetId":"123","reason":"..."}
func handleReport() http.HandlerFunc {
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

		var body struct {
			TargetType string `json:"targetType"`
			TargetID   string `json:"targetId"`
			Reason     string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		body.TargetType = strings.TrimSpace(body.TargetType)
		body.TargetID = strings.TrimSpace(body.TargetID)
		body.Reason = strings.TrimSpace(body.Reason)
		validTypes := map[string]bool{"post": true, "comment": true, "review": true, "file": true, "article": true}
		if !validTypes[body.TargetType] || body.TargetID == "" {
			apiErr(w, 400, "举报目标无效")
			return
		}
		if body.Reason == "" {
			apiErr(w, 400, "请填写举报原因")
			return
		}
		if len([]rune(body.Reason)) > 200 {
			apiErr(w, 400, "举报原因过长")
			return
		}
		// 防重复举报：同一用户对同一目标在「待处理」状态只允许举报一次
		var dup int
		authStore.db.QueryRow(
			"SELECT COUNT(*) FROM reports WHERE reporter_id=? AND target_type=? AND target_id=? AND status='待处理'",
			uid, body.TargetType, body.TargetID).Scan(&dup)
		if dup > 0 {
			apiErr(w, 400, "你已举报过该内容，请等待管理员处理")
			return
		}
		// 敏感词拦截举报原因
		if hits := hitWords(body.Reason); len(hits) > 0 {
			apiErr(w, 400, "举报原因包含敏感词: "+strings.Join(hits, "、"))
			return
		}
		// 反查目标作者，便于后台定位
		targetAuthor := lookupReportTargetAuthor(body.TargetType, body.TargetID)
		_, err := authStore.db.Exec(
			"INSERT INTO reports (target_type, target_id, target_author_id, reporter_id, reason, status, created_at) VALUES (?,?,?,?,?,?,?)",
			body.TargetType, body.TargetID, targetAuthor, uid, body.Reason, "待处理", time.Now().Format("2006-01-02 15:04:05"))
		if err != nil {
			apiErr(w, 500, "举报提交失败")
			return
		}
		apiJSON(w, 200, map[string]string{"message": "举报已提交，管理员会尽快处理"})
	}
}

// lookupReportTargetAuthor 反查被举报内容的作者ID（查不到返回0）
func lookupReportTargetAuthor(targetType, targetID string) int64 {
	id := parseID(targetID)
	if id == 0 {
		return 0
	}
	switch targetType {
	case "post":
		var uid int64
		authStore.db.QueryRow("SELECT user_id FROM posts WHERE id=?", id).Scan(&uid)
		return uid
	case "comment":
		var uid int64
		authStore.db.QueryRow("SELECT user_id FROM comments WHERE id=?", id).Scan(&uid)
		return uid
	case "review":
		var uid int64
		authStore.db.QueryRow("SELECT user_id FROM reviews WHERE id=?", id).Scan(&uid)
		return uid
	case "file":
		var uid int64
		authStore.db.QueryRow("SELECT uploader_id FROM files WHERE id=?", id).Scan(&uid)
		return uid
	case "article":
		var uid int64
		authStore.db.QueryRow("SELECT user_id FROM articles WHERE id=?", id).Scan(&uid)
		return uid
	}
	return 0
}