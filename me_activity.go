// me_activity.go — V10/V13 个人中心：我的内容聚合 + 草稿箱
// GET /api/me/activity  登录后返回：我的公开帖子/评价/资料/文章 + 草稿(帖子+文章) + 数量
package main

import (
	"net/http"
)

type myActivity struct {
	Posts    []map[string]any `json:"posts"`
	Reviews  []map[string]any `json:"reviews"`
	Files    []map[string]any `json:"files"`
	Articles []map[string]any `json:"articles"`
	Drafts   []map[string]any `json:"drafts"` // 草稿：帖子+文章混合，带类型标记
	Counts   map[string]int   `json:"counts"`
}

func handleMyActivity(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 {
			apiErr(w, 401, "请先登录")
			return
		}
		act := myActivity{Counts: map[string]int{}}

		// ---- 我的帖子（公开） ----
		rows, err := auth.db.Query(
			`SELECT p.id, p.forum, p.title, p.likes, p.created_at, p.is_anonymous
			 FROM posts p WHERE p.user_id=? AND p.status='正常' ORDER BY p.id DESC LIMIT 50`, uid)
		if err == nil {
			for rows.Next() {
				var id, likes int64
				var forum, title, created string
				var anon int
				rows.Scan(&id, &forum, &title, &likes, &created, &anon)
				act.Posts = append(act.Posts, map[string]any{
					"id": id, "forum": forum, "title": title, "likes": likes,
					"createdAt": created, "anonymous": anon == 1,
				})
			}
			rows.Close()
		}
		act.Counts["posts"] = len(act.Posts)

		// ---- 我的评价 ----
		rows, err = auth.db.Query(
			`SELECT r.id, r.course_code, r.rating, r.content, r.created_at, r.status
			 FROM reviews r WHERE r.user_id=? ORDER BY r.id DESC LIMIT 50`, uid)
		if err == nil {
			for rows.Next() {
				var id, rating int64
				var course, content, created, status string
				rows.Scan(&id, &course, &rating, &content, &created, &status)
				act.Reviews = append(act.Reviews, map[string]any{
					"id": id, "courseCode": course, "rating": rating,
					"content": content, "createdAt": created, "status": status,
				})
			}
			rows.Close()
		}
		act.Counts["reviews"] = len(act.Reviews)

		// ---- 我的资料 ----
		rows, err = auth.db.Query(
			`SELECT f.id, f.course_code, f.title, f.file_name, f.created_at, f.status
			 FROM files f WHERE f.uploader_id=? ORDER BY f.id DESC LIMIT 50`, uid)
		if err == nil {
			for rows.Next() {
				var id int64
				var course, title, fileName, created, status string
				rows.Scan(&id, &course, &title, &fileName, &created, &status)
				act.Files = append(act.Files, map[string]any{
					"id": id, "courseCode": course, "title": title,
					"fileName": fileName, "createdAt": created, "status": status,
				})
			}
			rows.Close()
		}
		act.Counts["files"] = len(act.Files)

		// ---- 我的文章（公开） ----
		rows, err = auth.db.Query(
			`SELECT a.id, a.title, a.category, a.created_at, a.status, a.reject_reason
			 FROM articles a WHERE a.user_id=? AND a.status!='draft' ORDER BY a.id DESC LIMIT 50`, uid)
		if err == nil {
			for rows.Next() {
				var id int64
				var title, category, created, status, reason string
				rows.Scan(&id, &title, &category, &created, &status, &reason)
				act.Articles = append(act.Articles, map[string]any{
					"id": id, "title": title, "category": category,
					"createdAt": created, "status": status, "rejectReason": reason,
				})
			}
			rows.Close()
		}
		act.Counts["articles"] = len(act.Articles)

		// ---- 我的草稿（帖子草稿 + 文章草稿，带 type 标记） ----
		rows, err = auth.db.Query(
			`SELECT p.id, 'post' AS dtype, p.title, p.forum AS cat, p.created_at
			 FROM posts p WHERE p.user_id=? AND p.status='draft' ORDER BY p.id DESC`, uid)
		if err == nil {
			for rows.Next() {
				var id int64
				var dtype, title, cat, created string
				rows.Scan(&id, &dtype, &title, &cat, &created)
				act.Drafts = append(act.Drafts, map[string]any{
					"id": id, "type": dtype, "title": title, "category": cat, "createdAt": created,
				})
			}
			rows.Close()
		}
		rows, err = auth.db.Query(
			`SELECT a.id, 'article' AS dtype, a.title, a.category, a.created_at
			 FROM articles a WHERE a.user_id=? AND a.status='draft' ORDER BY a.id DESC`, uid)
		if err == nil {
			for rows.Next() {
				var id int64
				var dtype, title, cat, created string
				rows.Scan(&id, &dtype, &title, &cat, &created)
				act.Drafts = append(act.Drafts, map[string]any{
					"id": id, "type": dtype, "title": title, "category": cat, "createdAt": created,
				})
			}
			rows.Close()
		}
		act.Counts["drafts"] = len(act.Drafts)

		// ---- 我的收藏 + 相册数量 (J6) ----
		var favCnt, albCnt int
		auth.db.QueryRow(`SELECT COUNT(*) FROM favorites WHERE user_id=?`, uid).Scan(&favCnt)
		auth.db.QueryRow(`SELECT COUNT(*) FROM albums WHERE user_id=?`, uid).Scan(&albCnt)
		act.Counts["favorites"] = favCnt
		act.Counts["albums"] = albCnt

		apiJSON(w, 200, act)
	}
}