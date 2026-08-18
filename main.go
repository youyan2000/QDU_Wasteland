// main.go — QDU Wasteland 后端入口
// 启动时加载三份课表到内存库
// 提供 /api/* 数据接口，并 serve public/ 前端静态文件
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var db *DB

// srv 注册 /api 路由并包 CSRF 防护（非安全方法校验同源）
func srv(pattern string, h http.HandlerFunc) { http.HandleFunc(pattern, csrfGuard(h)) }

func main() {
	// 初始化用户认证（SQLite）
	if err := initAuth(); err != nil {
		log.Fatalf("用户数据库初始化失败: %v", err)
	}
	// 文章旧分类迁移：老师→其他、生活方式→生活 (V9.1)
	migrateArticleCategories(authStore.db)
	// 专题表迁移 (F4)
	migrateTopicsTables(authStore.db)
	// 课程后台管理：overrides 表迁移 (F5)
	migrateCourseOverrides(authStore.db)
	// 班次管理：overrides/deletes 表迁移
	migrateOfferingTables(authStore.db)
	// 学院管理：colleges_admin 表迁移
	migrateCollegesAdmin(authStore.db)
	// 审计日志表迁移 (F6)
	migrateAuditLogs(authStore.db)
	// 意见箱表迁移 (G0)
	migrateOpinions(authStore.db)
	// 拉黑表迁移 (G5)
	migrateBlocklist(authStore.db)
	// 维护模式表迁移 (F7)
	migrateMaintenance(authStore.db)
	// 收藏 + 相册表迁移 (J6)
	migrateFavorites(authStore.db)
	// 文章封面列迁移 (J7)
	migrateArticleCover(authStore.db)

	// 后台：站长播种（仅当无可管理员时用 ADMIN_EMAIL/ADMIN_PASSWORD 创建）
	if email := os.Getenv("ADMIN_EMAIL"); email != "" {
		seedAdmin(authStore)
	} else {
		log.Println("ℹ️ 未设置 ADMIN_EMAIL，跳过站长播种；若尚无管理员请设置后重启。")
	}

	db = NewDB()

	// 课表 CSV 目录：优先从环境变量，默认 ../_csv
	csvDir := os.Getenv("CSV_DIR")
	if csvDir == "" {
		csvDir = filepath.Join("..", "_csv")
	}
	log.Printf("📚 正在加载课表数据: %s", csvDir)
	if err := db.loadAll(csvDir); err != nil {
		log.Fatalf("课表加载失败: %v", err)
	}
	log.Printf("✅ 已加载课程 %d 门, 开课班次 %d 条, 学院 %d 个",
		len(db.Courses), len(db.Offerings), len(db.CollegeSet))

	// 修正导入时的 name/code 错位（通用清洗）后重建排序
	fixNameCodeSwap(db)
	db.rebuildCourses()

	// 应用后台课程纠错覆盖 (F5.1)
	applyCourseOverrides(db, authStore.db)
	// 应用班次覆盖与删除
	applyOfferingOverrides(db, authStore.db)
	log.Printf("🟢 overrides OK")

	// J1：学院/专业全站统一（构建规范学院索引；course/offering 展示名归一到规范名）
	// 放在覆盖之后、最终 rebuild 之后，并在归一同时更新 byKey，确保后续 rebuild 不再回退
	initOrg()
	db.rebuildCourses()
	normalizeCollegeDisplay(db)
	log.Printf("🟢 normalizeCollegeDisplay OK (J1)")

	// 认证 API 路由
	srv("/api/register", rateLimitHandler("auth", handleRegister(authStore)))
	srv("/api/login", rateLimitHandler("auth", handleLogin(authStore)))
	srv("/api/logout", handleLogout())
	srv("/api/me", handleMe(authStore))
	srv("/api/me/activity", handleMyActivity(authStore))
	srv("/api/me/profile", rateLimitHandler("auth", handleUpdateProfile(authStore)))
	// 邮箱验证 (B2)
	srv("/api/email/verify", rateLimitHandler("auth", handleRequestEmailVerify()))
	srv("/api/email/verify/confirm", rateLimitHandler("auth", handleConfirmEmailVerify()))
	srv("/api/email/reset", rateLimitHandler("auth", handleRequestReset()))
	srv("/api/email/reset/confirm", rateLimitHandler("auth", handleConfirmReset()))

	// 课程 API 路由
	srv("/api/courses", handleCourses(db))
	// J1：学院/专业统一清单
	srv("/api/org", handleOrg(db))
	srv("/api/org/colleges", handleOrg(db))
	srv("/api/org/majors", handleOrg(db))
	// J5：籍贯省市数据
	srv("/api/regions/provinces", handleRegions())
	srv("/api/regions/cities", handleRegions())
	// 论坛 API (V4)
	srv("/api/forums", handleListForums())
	srv("/api/forum/posts", handleListPosts())
	srv("/api/forum/post/create", quotaHandler(rateLimitHandler("act", handleCreatePost())))
	srv("/api/forum/post", handlePostDetail())
	srv("/api/forum/comment", quotaHandler(rateLimitHandler("act", handleAddComment())))
	srv("/api/forum/like", handleLike())
	srv("/api/forum/delete", func(w http.ResponseWriter, r *http.Request) { apiErr(w, 501, "待实现") })
	srv("/api/forum/post/update", quotaHandler(rateLimitHandler("act", handleUpdatePost())))
	srv("/api/forum/post/delete", quotaHandler(rateLimitHandler("act", handleDeletePost())))
	srv("/api/articles/update", quotaHandler(rateLimitHandler("act", handleUpdateArticle())))
	srv("/api/articles/delete", quotaHandler(rateLimitHandler("act", handleDeleteArticle())))

	// 资料(文件)与评价 API (V3)
	srv("/api/files", handleListFilesAPI(db))
	srv("/api/upload", quotaHandler(rateLimitHandler("upload", handleUpload())))
	srv("/api/files/", handleFilesRouter())
	srv("/api/reviews", handleListReviews())
	srv("/api/reviews/add", quotaHandler(rateLimitHandler("act", handleAddReview())))
	srv("/api/courses/", handleCourses(db))
	srv("/api/tags", handleTags(db))
	srv("/api/semesters", handleSemesters(db))
	srv("/api/search", handleSearch(db))
	srv("/api/user/", handleUserProfile())
	srv("/api/stats", handleStats(db))

	// 后台管理 API (V5)
	srv("/api/admin/dashboard", handleAdminDashboard(authStore, db))
	srv("/api/admin/users", handleAdminUsers(authStore))
	srv("/api/admin/users/", handleAdminUsers(authStore))
	srv("/api/admin/reports", handleAdminReports(authStore))
	srv("/api/admin/reports/", handleAdminReports(authStore))
	// 用户封禁 (F2)
	srv("/api/admin/userban/", handleAdminUserBan(authStore))
	// 后台资料管理 (V6)
	srv("/api/admin/files", handleAdminListFiles(authStore))
	srv("/api/admin/files/", handleAdminFileStatus(authStore))
	// 图形验证码 (防脚本)
	srv("/api/captcha/", handleCaptchaImage())
	srv("/api/captcha", handleNewCaptcha())
	srv("/api/captcha/verify", handleVerifyCaptcha())
	// 前台举报 (V8)
	srv("/api/report", quotaHandler(rateLimitHandler("act", handleReport())))
	// 学校信息文章 (V9)
	srv("/api/articles", handleListArticles())
	srv("/api/articles/", handleListArticles())
	srv("/api/articles/create", quotaHandler(rateLimitHandler("act", handleCreateArticle())))
	// J7 文章封面
	srv("/api/article-cover", quotaHandler(rateLimitHandler("act", handleArticleCoverUpload())))
	srv("/api/cover-file/", handleCoverFile())
	srv("/api/admin/articles", handleAdminListArticles(authStore))
	srv("/api/admin/articles/", handleAdminListArticles(authStore))
	// 关键词审查管理 (V8)
	srv("/api/admin/censor", handleAdminCensor(authStore))
	srv("/api/admin/censor/", handleAdminCensor(authStore))
	// 站长公告 (D3)
	srv("/api/announcement", handleGetAnnouncement())
	// 通知中心 (D1)
	srv("/api/notifications", handleMyNotifications())
	srv("/api/notifications/read", handleReadNotifications())
	// 私信 (D2)
	srv("/api/messages", quotaHandler(rateLimitHandler("act", handleMyMessages())))
	srv("/api/messages/send", quotaHandler(rateLimitHandler("act", handleSendMessage())))
	srv("/api/messages/read", handleReadMessages())
	srv("/api/message/eligible", handleMessageEligible())
	// 收藏 + 相册 (J6)
	srv("/api/favorite/toggle", quotaHandler(rateLimitHandler("act", handleToggleFavorite())))
	srv("/api/favorites", handleMyFavorites())
	srv("/api/albums", handleAlbums())
	srv("/api/albums/create", quotaHandler(rateLimitHandler("act", handleAlbumCreate())))
	srv("/api/albums/delete", quotaHandler(rateLimitHandler("act", handleAlbumDelete())))
	srv("/api/albums/add-item", quotaHandler(rateLimitHandler("act", handleAlbumAddItem())))
	srv("/api/albums/items", handleAlbumItems())
	srv("/api/admin/announcement", handleAdminAnnouncement())
	// 后台评价管理 (V7)
	srv("/api/admin/reviews", handleAdminListReviews(authStore))
	srv("/api/admin/reviews/", handleAdminReviewStatus(authStore))
	srv("/api/admin/review", handleAdminReview(authStore))
	srv("/api/admin/review/", handleAdminReview(authStore))
	// 专题管理 (F4)
	srv("/api/topics", handleListTopics())
	srv("/api/topics/", handleListTopics())
	srv("/api/admin/topics", handleAdminCreateTopic(authStore))
	srv("/api/admin/topics/", handleAdminTopicArticles(authStore))
	srv("/api/admin/topic-articles", handleAdminTopicCandidateArticles(authStore))
	// 课程后台管理 (F5)
	srv("/api/admin/courses", handleAdminListCourses(db, authStore))
	srv("/api/admin/courses/", handleAdminListCourses(db, authStore))
	srv("/api/admin/courses/import", handleAdminImportCSV(db, authStore))
	srv("/api/admin/offerings", handleAdminListOfferings(db, authStore))
	srv("/api/admin/offerings/edit", handleAdminEditOffering(db, authStore))
	srv("/api/admin/offerings/delete", handleAdminDeleteOffering(db, authStore))
	srv("/api/admin/colleges", handleAdminListColleges(db, authStore))
	srv("/api/admin/college", handleAdminEditCollege(authStore))
	srv("/api/admin/audit", handleAdminAudit(authStore))
	srv("/api/admin/maintenance", handleAdminMaintenance(authStore))
	// 意见箱 (G0)
	srv("/api/opinion", quotaHandler(rateLimitHandler("act", handleSubmitOpinion(authStore))))
	// 拉黑 (G5)
	srv("/api/block", handleToggleBlock(authStore))
	srv("/api/block/list", handleBlockList(authStore))
	srv("/api/admin/opinions", handleAdminOpinions(authStore))
	srv("/api/admin/opinions/", handleAdminOpinions(authStore))

	// 静态前端 + 自定义 404（挂到 DefaultServeMux，与 srv() 注册的 /api 路由同源）
	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 禁止路径遍历
		p := r.URL.Path
		if strings.Contains(p, "..") {
			http.NotFound(w, r)
			return
		}
		// 若请求的静态文件存在则提供，否则返回品牌 404 页
		filePath := filepath.Join("public", filepath.Clean(p))
		fi, err := os.Stat(filePath)
		if err == nil && fi.IsDir() {
			// 请求的是目录（含根路径 /）：提供该目录的 index.html
			filePath = filepath.Join(filePath, "index.html")
			if fi2, err2 := os.Stat(filePath); err2 == nil && !fi2.IsDir() {
				http.ServeFile(w, r, filePath)
				return
			}
			http.ServeFile(w, r, "public/404.html")
			return
		}
		if err == nil && !fi.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, "public/404.html")
	}))
	// 维护模式中间件（普通访问者拦截，管理员/API 放行），包装 DefaultServeMux
	handler := maintenanceInterceptor(authStore.db, http.DefaultServeMux)

	port := os.Getenv("PORT")
	if port == "" { port = ":3000" }
	log.Printf("🟣 QDU Wasteland 已启动: http://localhost%s", port)
	if err := http.ListenAndServe(port, handler); err != nil {
		log.Fatal("服务器启动失败: ", err)
	}
}
