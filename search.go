// search.go — V11 全站搜索
// GET /api/search?q=xxx  返回 courses / posts / articles 三类匹配结果
// 课程来自内存库(名称/编号/院系/标签)，帖子与文章来自 SQLite LIKE（只含正常状态）
package main

import (
	"net/http"
	"strings"
)

type searchResult struct {
	Query    string          `json:"query"`
	Courses  []listCoursesDTO `json:"courses"`
	Posts    []postDTO        `json:"posts"`
	Articles []articleDTO     `json:"articles"`
}

func handleSearch(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		if q == "" {
			apiErr(w, 400, "缺少搜索词")
			return
		}
		res := searchResult{Query: q, Courses: []listCoursesDTO{}, Posts: []postDTO{}, Articles: []articleDTO{}}

		// ---- 课程（内存库）----
		if db != nil {
			for _, c := range db.Courses {
				if len(res.Courses) >= 20 {
					break
				}
				hay := strings.ToLower(c.Name + " " + c.Code + " " + c.College + " " + c.GeneralCat + " " + c.Tag)
				if strings.Contains(hay, q) {
					res.Courses = append(res.Courses, listCoursesDTO{
						Code: c.Code, Name: c.Name, College: c.College, Colleges: c.Colleges,
						Tag: c.Tag, Credit: c.Credit, Exam: c.ExamType, Nature: c.Nature,
						BigType: c.BigType, General: c.GeneralCat, Summary: c.FeaturedText,
					})
				}
			}
		}

		// ---- 帖子 ----
		prows, err := authStore.db.Query(
			`SELECT p.id, p.forum, p.title, p.is_anonymous, p.likes, p.created_at,
			        COALESCE(u.nickname,'匿名')
			 FROM posts p LEFT JOIN users u ON u.id=p.user_id
			 WHERE p.status='正常' AND COALESCE(u.banned,0)=0 AND (p.title LIKE ? OR p.content LIKE ?) ORDER BY p.id DESC LIMIT 20`,
			"%"+q+"%", "%"+q+"%")
		if err == nil {
			for prows.Next() {
				var p postDTO
				var anonI int
				var nick string
				prows.Scan(&p.ID, &p.Forum, &p.Title, &anonI, &p.Likes, &p.CreatedAt, &nick)
				p.Anonymous = anonI == 1
				if p.Anonymous || nick == "" { p.Author = "匿名" } else { p.Author = nick }
				res.Posts = append(res.Posts, p)
			}
			prows.Close()
		}

		// ---- 文章 ----
		arows, err := authStore.db.Query(
			`SELECT a.id, a.title, a.category, a.content, a.cover, a.is_anonymous, a.status, a.created_at,
			        COALESCE(u.nickname,'')
			 FROM articles a LEFT JOIN users u ON u.id=a.user_id
			 WHERE a.status='正常' AND COALESCE(u.banned,0)=0 AND (a.title LIKE ? OR a.content LIKE ?) ORDER BY a.id DESC LIMIT 20`,
			"%"+q+"%", "%"+q+"%")
		if err == nil {
			var aCover string
			for arows.Next() {
				var a articleDTO
				var anonI int
				var nick string
				arows.Scan(&a.ID, &a.Title, &a.Category, &a.Content, &aCover, &anonI, &a.Status, &a.CreatedAt, &nick)
				a.Cover = aCover
				a.Anonymous = anonI == 1
				if a.Anonymous || nick == "" { a.Author = "匿名" } else { a.Author = nick }
				if len(a.Content) > 80 { a.Content = a.Content[:80] + "…" }
				res.Articles = append(res.Articles, a)
			}
			arows.Close()
		}

		apiJSON(w, 200, res)
	}
}