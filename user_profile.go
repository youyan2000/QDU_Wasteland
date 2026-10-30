// user_profile.go — V13 (D4/E4) 公开个人主页
// GET /api/user/{id}  访客可看：昵称/学院/专业 + TA公开发布的帖子与文章
package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// privacyFlag 读取一个隐私开关：key 存在则用用户的选择（0/1），不存在才用默认值。
// 这条语义是内容显隐开关能否生效的关键（审计 P1-3 B）。
// 数字类型要全兼容：JSON 解出来是 float64，但内部构造的 map 可能是 int，
// 只认 float64 会让「显式关闭」悄悄退回默认值。
func privacyFlag(priv map[string]any, key string, def float64) float64 {
	v, ok := priv[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f
		}
	case bool:
		if n {
			return 1
		}
		return 0
	case string:
		switch n {
		case "1", "true":
			return 1
		case "0", "false":
			return 0
		}
	}
	return def
}

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
		// 隐私设置：决定哪些字段对外展示（0=不公开；key 缺失时取默认值）
		priv := getUserPrivacy(authStore.db, uid)
		// flag 区分「用户显式关闭(0)」和「从未设置(缺 key)」——
		// 早期实现统一 `if x == 0 { x = 1 }`，把"我选择隐藏"也重置成公开，
		// 导致内容显隐开关完全失效（P1-3 B）。
		showAge := privacyFlag(priv, "age", 0)
		showGender := privacyFlag(priv, "gender", 0)
		showCollege := privacyFlag(priv, "college", 1)
		showMajor := privacyFlag(priv, "major", 1)
		showNative := privacyFlag(priv, "nativePlace", 1)
		showBirthday := privacyFlag(priv, "birthday", 0)
		showWechat := privacyFlag(priv, "wechat", 0)
		showQQ := privacyFlag(priv, "qq", 0)
		showPhone := privacyFlag(priv, "phone", 0)
		showSocial := privacyFlag(priv, "social", 0)
		// 内容显隐：帖子/文章/评价/收藏/相册（默认公开=1）
		showPosts := privacyFlag(priv, "posts", 1)
		showArticles := privacyFlag(priv, "articles", 1)
		showReviews := privacyFlag(priv, "reviews", 1)
		showFavorites := privacyFlag(priv, "favorites", 1)
		showAlbums := privacyFlag(priv, "albums", 1)
		pubCollege, pubMajor := college, major
		if showCollege == 0 { pubCollege = "" }
		if showMajor == 0 { pubMajor = "" }
		// 年龄/性别/籍贯：默认就不公开，必须按开关打码（P1-3 A）。
		pubAge := age
		if showAge == 0 { pubAge = 0 }
		pubGender := gender
		if showGender == 0 { pubGender = "" }
		pubNative := nativePlace
		if showNative == 0 { pubNative = "" }
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
				"age": pubAge, "gender": pubGender, "nativePlace": pubNative,
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