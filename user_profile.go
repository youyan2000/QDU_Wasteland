// user_profile.go — V13 (D4/E4) 公开个人主页
// GET /api/user/{id}  访客可看：昵称/学院/专业 + TA公开发布的帖子与文章
package main

import (
	"net/http"
	"strings"
)

func handleUserProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := strings.TrimPrefix(r.URL.Path, "/api/user/")
		uid := parseID(idStr)
		if uid == 0 {
			apiErr(w, 400, "无效用户")
			return
		}
		var nickname, college, major, gender, birthday, nativePlace, wechat, qq, phone, social string
		var age int
		var banned int
		err := authStore.db.QueryRow(
			"SELECT nickname, college, major, COALESCE(gender,''), COALESCE(age,0), COALESCE(birthday,''), COALESCE(native_place,''), COALESCE(wechat,''), COALESCE(qq,''), COALESCE(phone,''), COALESCE(social,''), COALESCE(banned,0) FROM users WHERE id=?", uid).
			Scan(&nickname, &college, &major, &gender, &age, &birthday, &nativePlace, &wechat, &qq, &phone, &social, &banned)
		if err != nil {
			apiErr(w, 404, "用户不存在")
			return
		}
		if nickname == "" {
			nickname = "用户"
		}
		if banned == 1 {
			apiJSON(w, 200, map[string]any{"banned": true, "nickname": nickname})
			return
		}
		// 隐私设置：决定哪些字段对外展示（0=不公开）
		priv := getUserPrivacy(authStore.db, uid)
		showAge, _ := priv["age"].(float64)
		showGender, _ := priv["gender"].(float64)
		showCollege, _ := priv["college"].(float64)
		showMajor, _ := priv["major"].(float64)
		showNative, _ := priv["nativePlace"].(float64)
		showBirthday, _ := priv["birthday"].(float64)
		showWechat, _ := priv["wechat"].(float64)
		showQQ, _ := priv["qq"].(float64)
		showPhone, _ := priv["phone"].(float64)
		showSocial, _ := priv["social"].(float64)
		// 内容显隐：帖子/文章/评价/收藏/相册（默认公开=1）
		showPosts, _ := priv["posts"].(float64)
		showArticles, _ := priv["articles"].(float64)
		showReviews, _ := priv["reviews"].(float64)
		showFavorites, _ := priv["favorites"].(float64)
		showAlbums, _ := priv["albums"].(float64)
		if showPosts == 0 { showPosts = 1 } // 默认公开
		if showArticles == 0 { showArticles = 1 }
		if showReviews == 0 { showReviews = 1 }
		if showFavorites == 0 { showFavorites = 1 }
		if showAlbums == 0 { showAlbums = 1 }
		pubCollege, pubMajor := college, major
		if showCollege == 0 { pubCollege = "" }
		if showMajor == 0 { pubMajor = "" }
		// 联系方式/生日：默认不公开（隐私控制）
		pubBirthday, pubWechat, pubQQ, pubPhone, pubSocial := birthday, wechat, qq, phone, social
		if showBirthday == 0 { pubBirthday = "" }
		if showWechat == 0 { pubWechat = "" }
		if showQQ == 0 { pubQQ = "" }
		if showPhone == 0 { pubPhone = "" }
		if showSocial == 0 { pubSocial = "" }

		var postCount, articleCount int
		authStore.db.QueryRow("SELECT COUNT(*) FROM posts WHERE user_id=? AND status='正常'", uid).Scan(&postCount)
		authStore.db.QueryRow("SELECT COUNT(*) FROM articles WHERE user_id=? AND status='正常'", uid).Scan(&articleCount)

		// TA 的公开帖子（隐私关闭则列表为空）
		var posts []map[string]any
		if showPosts == 1 {
			pr, _ := authStore.db.Query(
				`SELECT p.id, p.forum, p.title, p.likes, p.created_at
				 FROM posts p WHERE p.user_id=? AND p.status='正常' ORDER BY p.id DESC LIMIT 20`, uid)
			if pr != nil {
				for pr.Next() {
					var id, likes int64
					var forum, title, created string
					pr.Scan(&id, &forum, &title, &likes, &created)
					posts = append(posts, map[string]any{"id": id, "forum": forum, "title": title, "likes": likes, "createdAt": created})
				}
				pr.Close()
			}
		}

		// TA 的公开文章（隐私关闭则列表为空）
		var articles []map[string]any
		if showArticles == 1 {
			ar, _ := authStore.db.Query(
				`SELECT a.id, a.title, a.category, a.created_at
				 FROM articles a WHERE a.user_id=? AND a.status='正常' ORDER BY a.id DESC LIMIT 20`, uid)
			if ar != nil {
				for ar.Next() {
					var id int64
					var title, cat, created string
					ar.Scan(&id, &title, &cat, &created)
					articles = append(articles, map[string]any{"id": id, "title": title, "category": cat, "createdAt": created})
				}
				ar.Close()
			}
		}

		apiJSON(w, 200, map[string]any{
			"user": map[string]any{
				"id": uid, "nickname": nickname,
				"college": pubCollege, "major": pubMajor,
				"age": age, "gender": gender, "nativePlace": nativePlace,
				"birthday": pubBirthday, "wechat": pubWechat, "qq": pubQQ,
				"phone": pubPhone, "social": pubSocial,
				"privacy": map[string]any{
					"age": showAge, "gender": showGender, "college": showCollege,
					"major": showMajor, "nativePlace": showNative,
					"birthday": showBirthday, "wechat": showWechat, "qq": showQQ,
					"phone": showPhone, "social": showSocial,
					"posts": showPosts, "articles": showArticles, "reviews": showReviews,
					"favorites": showFavorites, "albums": showAlbums,
				},
				"postCount": postCount, "articleCount": articleCount,
			},
			"posts": posts, "articles": articles,
		})
	}
}