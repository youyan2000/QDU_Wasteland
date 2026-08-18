// api_forum.go — 论坛 API (V4)
// 广场：trade/paper/help/friend
// 接口：
//   GET /api/forums                列出四广场
//   GET /api/forum/posts?f=<slug>&sort=latest|hot   列出某广场帖子
//   POST /api/forum/posts          发帖（登录）
//   GET /api/forum/post?id=xxx     帖子详情+评论
//   POST /api/forum/comment        评论（登录）
//   POST /api/forum/like?id=xxx    点赞（粗略计数）
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type forumInfo struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Desc  string `json:"desc"`
	Count int    `json:"count"`
}

func handleListForums() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := authStore.db.Query("SELECT slug,name,desc FROM forums ORDER BY id")
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		var out []forumInfo
		for rows.Next() {
			var f forumInfo
			rows.Scan(&f.Slug, &f.Name, &f.Desc)
			out = append(out, f)
		}
		rows.Close() // 关键：释放连接后再查库（单连接否则会死锁）
		// 每个广场帖数
		for i := range out {
			var n int
			authStore.db.QueryRow("SELECT COUNT(*) FROM posts WHERE forum=? AND status='正常'", out[i].Slug).Scan(&n)
			out[i] = forumInfo{Slug: out[i].Slug, Name: out[i].Name, Desc: out[i].Desc, Count: n}
		}
		apiJSON(w, 200, map[string]any{"forums": out})
	}
}

type postDTO struct {
	ID        int64  `json:"id"`
	Forum     string `json:"forum"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	AuthorID  int64  `json:"authorId,omitempty"`
	Anonymous bool   `json:"anonymous"`
	Likes     int64  `json:"likes"`
	Liked     bool   `json:"liked"`
	CommentN  int    `json:"commentCount"`
	CreatedAt string `json:"createdAt"`
}

func handleListPosts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f := r.URL.Query().Get("f")
		sort := r.URL.Query().Get("sort") // latest|hot
		q := `SELECT p.id, p.forum, p.title, p.is_anonymous, p.likes, p.created_at,
			COALESCE(u.nickname,'匿名'), p.user_id, (SELECT COUNT(*) FROM comments c WHERE c.post_id=p.id)
			FROM posts p LEFT JOIN users u ON u.id=p.user_id`
		args := []any{}
		if f != "" {
			q += " WHERE p.forum=? AND p.status='正常' AND COALESCE(u.banned,0)=0"
			args = append(args, f)
		} else {
			q += " WHERE p.status='正常' AND COALESCE(u.banned,0)=0"
		}
		if sort == "hot" {
			q += " ORDER BY (p.likes + (SELECT COUNT(*) FROM comments c WHERE c.post_id=p.id) * 2) DESC, p.id DESC"
		} else {
			q += " ORDER BY p.id DESC"
		}
		// C4 分页
		var page, size int
		fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
		fmt.Sscanf(r.URL.Query().Get("size"), "%d", &size)
		if page < 1 { page = 1 }
		if size < 1 || size > 50 { size = 15 }
		// 查总数
		var total int
		countQ := "SELECT COUNT(*) FROM posts WHERE status='正常'"
		cargs := []any{}
		if f != "" { countQ += " AND forum=?"; cargs = append(cargs, f) }
		authStore.db.QueryRow(countQ, cargs...).Scan(&total)
		// LIMIT/OFFSET
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", size, (page-1)*size)

		// 先查当前用户已赞集合（在打开主 rows 之前，单连接否则会死锁）
		myLikes := map[int64]bool{}
		if uid := currentUserID(r); uid != 0 {
			lrows, _ := authStore.db.Query("SELECT post_id FROM post_likes WHERE user_id=?", uid)
			for lrows.Next() { var pid int64; lrows.Scan(&pid); myLikes[pid] = true }
			lrows.Close()
		}
		// 再开主 rows
		rows, err := authStore.db.Query(q, args...)
		if err != nil {
			apiErr(w, 500, "查询失败")
			return
		}
		defer rows.Close()
		var out []postDTO
		for rows.Next() {
			var p postDTO
			var anonI int
			var nickname string
			var ownerID int64
			rows.Scan(&p.ID, &p.Forum, &p.Title, &anonI, &p.Likes, &p.CreatedAt, &nickname, &ownerID, &p.CommentN)
			p.Anonymous = anonI == 1
			if !p.Anonymous { p.AuthorID = ownerID }
			if p.Anonymous || nickname == "" { p.Author = "匿名" } else { p.Author = nickname }
			p.Liked = myLikes[p.ID]
			out = append(out, p)
		}
		apiJSON(w, 200, map[string]any{"posts": out, "forum": f, "total": total, "page": page, "pageSize": size})
	}
}

func handleCreatePost() http.HandlerFunc {
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
		if currentUserMuted(r) {
			apiErr(w, 403, "该账号已被限制发布，无法进行此操作")
			return
		}

		var body struct {
			Forum     string `json:"forum"`
			Title     string `json:"title"`
			Content   string `json:"content"`
			Anonymous bool   `json:"anonymous"`
			Captcha   string `json:"captcha"`
			CaptchaID string `json:"captchaId"`
			Status    string `json:"status"` // 可选：draft（存草稿，跳过验证码/邮箱验证）
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		body.Title = strings.TrimSpace(body.Title)
		body.Content = strings.TrimSpace(body.Content)
		isDraft := body.Status == "draft"
		// 草稿：允许空标题/内容，跳过验证码和邮箱验证
		if !isDraft {
			// 图形验证码校验 (V9)
			if !verifyCaptchaCode(body.CaptchaID, strings.TrimSpace(body.Captcha)) {
				apiErr(w, 400, "验证码错误，请重试")
				return
			}
			if body.Forum == "" || body.Title == "" {
				apiErr(w, 400, "请填写广场和标题")
				return
			}
			// 邮箱验证（斐波那契触发）
			if requireEmailVerified(uid) {
				apiErr(w, 403, "请先验证邮箱后再继续发布（验证码已可发送，见 /api/email/verify）")
				return
			}
		}
		// 关键词自动审查 (F1)：命中敏感词 → 落到待复核队列，审核通过后才公开
		status := "正常"
		if isDraft {
			status = "draft"
		} else if hits := hitWords(body.Title + " " + body.Content); len(hits) > 0 {
			status = "待复核"
		}
		_, err := authStore.db.Exec(
			"INSERT INTO posts (forum,user_id,title,content,is_anonymous,status,created_at) VALUES (?,?,?,?,?,?,?)",
			body.Forum, uid, body.Title, body.Content, b2i(body.Anonymous), status, time.Now().Format("2006-01-02 15:04:05"))
		if err != nil {
			apiErr(w, 500, "发帖失败")
			return
		}
		if isDraft {
			apiJSON(w, 200, map[string]string{"message": "已存为草稿"})
			return
		}
		if status == "待复核" {
			apiJSON(w, 200, map[string]string{"message": "内容含敏感词，已进入待审队列，审核通过后展示"})
			return
		}
		apiJSON(w, 200, map[string]string{"message": "发帖成功"})
	}
}

type commentDTO struct {
	ID        int64  `json:"id"`
	Author    string `json:"author"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
	ParentID  int64  `json:"parentId"`
}

func handlePostDetail() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := parseID(r.URL.Query().Get("id"))
		var p struct {
			ID int64; Forum, Title, Content string
		}
		var pStatus string
		var anonI int
		var uid int64
		var created string
		err := authStore.db.QueryRow("SELECT id,forum,title,content,user_id,is_anonymous,status,created_at FROM posts WHERE id=?", id).
			Scan(&p.ID, &p.Forum, &p.Title, &p.Content, &uid, &anonI, &pStatus, &created)
		if err != nil {
			apiErr(w, 404, "帖子不存在")
			return
		}
		// F2 封禁作者内容隐藏
		var authorBanned int
		authStore.db.QueryRow("SELECT COALESCE(banned,0) FROM users WHERE id=?", uid).Scan(&authorBanned)
		if authorBanned == 1 {
			apiErr(w, 404, "帖子不存在")
			return
		}
		// C1 草稿箱：草稿仅作者或管理员可见
		if pStatus == "draft" {
			me := currentUserID(r)
			if me != uid {
				canSee := false
				if me != 0 {
					u, _ := authStore.GetByID(me)
					canSee = u != nil && u.IsAdmin == 1
				}
				if !canSee {
					apiErr(w, 404, "帖子不存在")
					return
				}
			}
		}
		// 作者名
		var author string
		if anonI == 1 {
			author = "匿名"
		} else {
			authStore.db.QueryRow("SELECT COALESCE(nickname,'') FROM users WHERE id=?", uid).Scan(&author)
			if author == "" { author = "匿名" }
		}
		// 评论
		rows, _ := authStore.db.Query(
			`SELECT c.id, c.content, c.is_anonymous, c.created_at, c.parent_id, COALESCE(u.nickname,'')
			 FROM comments c LEFT JOIN users u ON u.id=c.user_id WHERE c.post_id=? AND COALESCE(u.banned,0)=0 ORDER BY c.id`, id)
		var comments []commentDTO
		for rows.Next() {
			var cm commentDTO
			var caI int
			var nick string
			rows.Scan(&cm.ID, &cm.Content, &caI, &cm.CreatedAt, &cm.ParentID, &nick)
			if caI == 1 || nick == "" { cm.Author = "匿名" } else { cm.Author = nick }
			comments = append(comments, cm)
		}
		rows.Close() // 关键：释放连接后再查 likes（单连接否则会死锁）
		// 点赞数 + 当前用户是否已赞
		var likeN int64
		authStore.db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id=?", id).Scan(&likeN)
		liked := false
		if uid := currentUserID(r); uid != 0 {
			var n int
			authStore.db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id=? AND user_id=?", id, uid).Scan(&n)
			liked = n > 0
		}
		apiJSON(w, 200, map[string]any{
			"id": p.ID, "forum": p.Forum, "title": p.Title, "content": p.Content,
			"author": author, "anonymous": anonI == 1, "createdAt": created,
			"likes": likeN, "liked": liked, "ownerId": uid,
			"comments": comments,
		})
	}
}

func handleAddComment() http.HandlerFunc {
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
		if currentUserMuted(r) {
			apiErr(w, 403, "该账号已被限制发布，无法进行此操作")
			return
		}
		// 邮箱验证（斐波那契触发）
		if requireEmailVerified(uid) {
			apiErr(w, 403, "请先验证邮箱后再继续发布（验证码见 /api/email/verify）")
			return
		}

		var body struct {
			PostID    int64  `json:"postId"`
			Content   string `json:"content"`
			Anonymous bool   `json:"anonymous"`
			ParentID  int64  `json:"parentId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		body.Content = strings.TrimSpace(body.Content)
		if body.Content == "" {
			apiErr(w, 400, "评论不能为空")
			return
		}
		// 关键词自动审查拦截 (V8)
		if hits := hitWords(body.Content); len(hits) > 0 {
			apiErr(w, 400, "内容包含敏感词，已被自动拦截: "+strings.Join(hits, "、"))
			return
		}
		_, err := authStore.db.Exec(
			"INSERT INTO comments (post_id,user_id,content,is_anonymous,created_at,parent_id) VALUES (?,?,?,?,?,?)",
			body.PostID, uid, body.Content, b2i(body.Anonymous), time.Now().Format("2006-01-02 15:04:05"), body.ParentID)
		if err != nil {
			apiErr(w, 500, "评论失败")
			return
		}
		// D1 通知帖子作者（楼中楼回复不重复通知作者）
		if body.ParentID == 0 {
			notifyPostAuthor(body.PostID, uid, "comment", "评论了你的帖子")
		}
		apiJSON(w, 200, map[string]string{"message": "评论成功"})
	}
}

func handleLike() http.HandlerFunc {
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
		if currentUserMuted(r) {
			apiErr(w, 403, "该账号已被限制发布，无法进行此操作")
			return
		}

		id := parseID(r.URL.Query().Get("id"))
		if id == 0 {
			apiErr(w, 400, "缺少帖子")
			return
		}
		// 切换式点赞：未赞则赞，已赞则取消
		var existing int
		authStore.db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id=? AND user_id=?", id, uid).Scan(&existing)
		liked := true
		if existing == 0 {
			authStore.db.Exec("INSERT INTO post_likes (post_id,user_id) VALUES (?,?)", id, uid)
			notifyPostAuthor(id, uid, "like", "赞了你的帖子")
		} else {
			authStore.db.Exec("DELETE FROM post_likes WHERE post_id=? AND user_id=?", id, uid)
			liked = false
		}
		// 同步 posts.likes 计数
		var n int
		authStore.db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id=?", id).Scan(&n)
		authStore.db.Exec("UPDATE posts SET likes=? WHERE id=?", n, id)
		apiJSON(w, 200, map[string]any{"liked": liked, "likes": n})
	}
}

func parseID(s string) int64 {
	var n int64
	for _, c := range s {
		if c >= '0' && c <= '9' { n = n*10 + int64(c-'0') } else { return 0 }
	}
	return n
}
