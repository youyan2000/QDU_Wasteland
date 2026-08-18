// content_update.go — V13 (C2) 帖子和文章的编辑/删除（作者+管理员）
// 语义：删除=转草稿(draft)、发布=转正常(normal)、编辑=更新内容后按 status 决定落草稿或公开
package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// canModifyContent 判断能否修改：作者本人 或 管理员
func canModifyContent(ownerID int64, r *http.Request) bool {
	me := currentUserID(r)
	if me == 0 {
		return false
	}
	if me == ownerID {
		return true
	}
	u, _ := authStore.GetByID(me)
	return u != nil && u.IsAdmin == 1
}

// normalizeStatus 校验状态值：只能 draft 或 正常
func normalizeStatus(s string) string {
	s = strings.TrimSpace(s)
	if s != "draft" && s != "正常" {
		return ""
	}
	return s
}

// handleUpdatePost 帖子编辑/发布
func handleUpdatePost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID     int64  `json:"id"`
			Title  string `json:"title"`
			Content string `json:"content"`
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		st := normalizeStatus(body.Status)
		if st == "" {
			apiErr(w, 400, "状态无效")
			return
		}
		var owner int64
		err := authStore.db.QueryRow("SELECT user_id FROM posts WHERE id=?", body.ID).Scan(&owner)
		if err != nil {
			apiErr(w, 404, "帖子不存在")
			return
		}
		if !canModifyContent(owner, r) {
			apiErr(w, 403, "无权限编辑该帖子")
			return
		}
		body.Title = strings.TrimSpace(body.Title)
		body.Content = strings.TrimSpace(body.Content)
		msg := "已保存"
		if st == "正常" { msg = "已发布" } else { msg = "已存为草稿" }
		if body.Title == "" && body.Content == "" {
			// 仅改状态（草稿箱的发布/撤回），保留原文
			_, err = authStore.db.Exec("UPDATE posts SET status=? WHERE id=?", st, body.ID)
		} else {
			if body.Title == "" {
				apiErr(w, 400, "标题不能为空")
				return
			}
			if hits := hitWords(body.Title + " " + body.Content); len(hits) > 0 {
				apiErr(w, 400, "内容包含敏感词: "+strings.Join(hits, "、"))
				return
			}
			_, err = authStore.db.Exec("UPDATE posts SET title=?, content=?, status=? WHERE id=?",
				body.Title, body.Content, st, body.ID)
		}
		if err != nil {
			apiErr(w, 500, "保存失败")
			return
		}
		apiJSON(w, 200, map[string]any{"message": msg, "status": st})
	}
}

// handleDeletePost 帖子删除：已发布→转草稿；已是草稿→彻底删除
func handleDeletePost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct{ ID int64 `json:"id"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		var owner int64
		var status string
		err := authStore.db.QueryRow("SELECT user_id, status FROM posts WHERE id=?", body.ID).Scan(&owner, &status)
		if err != nil {
			apiErr(w, 404, "帖子不存在")
			return
		}
		if !canModifyContent(owner, r) {
			apiErr(w, 403, "无权限删除该帖子")
			return
		}
		if status == "draft" {
			// 草稿箱内删除 = 彻底删除（含评论与点赞）
			_, _ = authStore.db.Exec("DELETE FROM comments WHERE post_id=?", body.ID)
			_, _ = authStore.db.Exec("DELETE FROM post_likes WHERE post_id=?", body.ID)
			_, err = authStore.db.Exec("DELETE FROM posts WHERE id=?", body.ID)
			if err != nil {
				apiErr(w, 500, "删除失败")
				return
			}
			apiJSON(w, 200, map[string]any{"message": "草稿已彻底删除", "status": "deleted"})
			return
		}
		_, err = authStore.db.Exec("UPDATE posts SET status='draft' WHERE id=?", body.ID)
		if err != nil {
			apiErr(w, 500, "删除失败")
			return
		}
		apiJSON(w, 200, map[string]any{"message": "帖子已撤回草稿箱", "status": "draft"})
	}
}

// handleUpdateArticle 文章编辑/发布
func handleUpdateArticle() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID       int64  `json:"id"`
			Title    string `json:"title"`
			Category string `json:"category"`
			Content  string `json:"content"`
			Status   string `json:"status"`
			Cover    string `json:"cover"` // J7
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		st := normalizeStatus(body.Status)
		if st == "" {
			apiErr(w, 400, "状态无效")
			return
		}
		var owner int64
		err := authStore.db.QueryRow("SELECT user_id FROM articles WHERE id=?", body.ID).Scan(&owner)
		if err != nil {
			apiErr(w, 404, "文章不存在")
			return
		}
		if !canModifyContent(owner, r) {
			apiErr(w, 403, "无权限编辑该文章")
			return
		}
		body.Title = strings.TrimSpace(body.Title)
		body.Content = strings.TrimSpace(body.Content)
		msg := "已保存"
		if st == "正常" { msg = "已发布" } else { msg = "已存为草稿" }
		if body.Title == "" && body.Content == "" {
			_, err = authStore.db.Exec("UPDATE articles SET status=? WHERE id=?", st, body.ID)
		} else {
			if body.Title == "" || body.Content == "" {
				apiErr(w, 400, "标题和内容不能为空")
				return
			}
			if hits := hitWords(body.Title + " " + body.Content); len(hits) > 0 {
				apiErr(w, 400, "内容包含敏感词: "+strings.Join(hits, "、"))
				return
			}
			if body.Category == "" {
				body.Category = "生活"
			}
			_, err = authStore.db.Exec("UPDATE articles SET title=?, category=?, content=?, cover=?, status=? WHERE id=?",
				body.Title, body.Category, body.Content, strings.TrimSpace(body.Cover), st, body.ID)
		}
		if err != nil {
			apiErr(w, 500, "保存失败")
			return
		}
		apiJSON(w, 200, map[string]any{"message": msg, "status": st})
	}
}

// handleDeleteArticle 文章删除：已发布→转草稿；已是草稿→彻底删除
func handleDeleteArticle() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct{ ID int64 `json:"id"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		var owner int64
		var status string
		err := authStore.db.QueryRow("SELECT user_id, status FROM articles WHERE id=?", body.ID).Scan(&owner, &status)
		if err != nil {
			apiErr(w, 404, "文章不存在")
			return
		}
		if !canModifyContent(owner, r) {
			apiErr(w, 403, "无权限删除该文章")
			return
		}
		if status == "draft" {
			// 草稿箱内删除 = 彻底删除
			_, err = authStore.db.Exec("DELETE FROM articles WHERE id=?", body.ID)
			if err != nil {
				apiErr(w, 500, "删除失败")
				return
			}
			apiJSON(w, 200, map[string]any{"message": "草稿已彻底删除", "status": "deleted"})
			return
		}
		_, err = authStore.db.Exec("UPDATE articles SET status='draft' WHERE id=?", body.ID)
		if err != nil {
			apiErr(w, 500, "删除失败")
			return
		}
		apiJSON(w, 200, map[string]any{"message": "文章已撤回草稿箱", "status": "draft"})
	}
}