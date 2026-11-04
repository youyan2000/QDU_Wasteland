// main.go — QDU Wasteland 后端入口
// 启动时加载三份课表到内存库
// 提供 /api/* 数据接口，并 serve public/ 前端静态文件
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"wasteland/internal/tools/history"
)

// logWriter 记录状态码与响应字节数，供同步接口的访问日志使用。
type logWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *logWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *logWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

var db *DB

// srv 注册 /api 路由并包 CSRF 防护（非安全方法校验同源）
func srv(pattern string, h http.HandlerFunc) { http.HandleFunc(pattern, csrfGuard(h)) }

func main() {
	// 先确定站点根目录：public/、tool-data/、dist/、_csv/、qdu-auth.db 全部相对它解析。
	// 找不到就直接退出，避免"工具能用、页面全 404"的半空壳状态。
	base, err := resolveBaseDir()
	if err != nil {
		log.Fatalf("❌ %v", err)
	}
	baseDir = base
	log.Printf("🗂 站点根目录: %s", baseDir)

	// 初始化用户认证（SQLite）
	if err := initAuth(); err != nil {
		log.Fatalf("用户数据库初始化失败: %v", err)
	}
	// 文章旧分类迁移：老师→其他、生活方式→生活 (V9.1)
	migrateArticleCategories(authStore.db)
	// 文章阅读量列迁移
	migrateArticleViews(authStore.db)
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
	// 禁言列迁移
	migrateMuted(authStore.db)
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

	// 课表 CSV 目录：优先从环境变量，默认站点根目录上一级的 _csv
	csvDir := os.Getenv("CSV_DIR")
	if csvDir == "" {
		csvDir = defaultCSVDir()
	}
	log.Printf("📚 正在加载课表数据: %s", csvDir)
	if err := db.loadAll(csvDir); err != nil {
		log.Fatalf("❌ 课表加载失败（CSV_DIR=%s）：%v\n"+
			"   课表 CSV 不在代码仓里。请把 CSV_DIR 指向包含 *.csv 的目录（默认 %s）",
			csvDir, err, defaultCSVDir())
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
	// 个人资料/对外展示保存：不做限流（登录态轻操作，csrfGuard 已防护）；改密码保留 act 桶(30/h)防爆破
	srv("/api/me/profile", handleUpdateProfile(authStore))
	srv("/api/me/password", quotaHandler(rateLimitHandler("act", handleChangePassword(authStore))))
	// 头像上传/访问 (V15)
	srv("/api/avatar/upload", quotaHandler(rateLimitHandler("act", handleAvatarUpload())))
	http.HandleFunc("/avatar-file/", handleAvatarFile())
	// 邮箱验证 (B2)：用独立 mail 档限流（10 次/小时），不与登录(auth 5次)共用配额
	srv("/api/email/verify", rateLimitHandler("mail", handleRequestEmailVerify()))
	srv("/api/email/verify/confirm", rateLimitHandler("mail", handleConfirmEmailVerify()))
	srv("/api/email/reset", rateLimitHandler("mail", handleRequestReset()))
	srv("/api/email/reset/confirm", rateLimitHandler("mail", handleConfirmReset()))

	// 登录用户的本站浏览足迹（不接受客户端传入 user ID）。
	srv("/api/visit", quotaHandler(rateLimitHandler("act", handleRecordVisit())))

	// 全站随机单词（词库编译进二进制，见 internal/wordbank）。
	srv("/api/words/random", handleRandomWords())

	// 浏览器扩展自动同步：用设备令牌鉴权，不依赖网页会话，因此不走 csrfGuard。
	// 每个 IP 每小时最多 120 次同步请求，避免令牌泄露后被当成上传通道刷。
	syncHandler := history.SyncHandler(authStore.db, 8<<20)
	http.HandleFunc("/api/history/sync", func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		lw := &logWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			// 同步是排查"扩展没反应"的关键路径，必须留下可检索的访问日志。
			log.Printf("history/sync %s %d %dB %s", r.Method, lw.status, lw.bytes, time.Since(started).Round(time.Millisecond))
		}()
		if ok, _ := rateLimitOK(fpKey(r)+"|sync", 120); !ok {
			w.Header().Set("Retry-After", "600")
			apiErr(lw, 429, "同步过于频繁，请稍后再试")
			return
		}
		syncHandler(lw, r)
	})

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
	srv("/api/course-categories", handleCourseCategories())
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
	srv("/api/admin/userban/mute/", handleAdminUserMute(authStore))
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
	srv("/api/admin/announcement", handleAdminAnnouncement())
	srv("/api/admin/announcement/", handleAdminAnnouncement())
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
	srv("/api/albums/remove-item", quotaHandler(rateLimitHandler("act", handleAlbumRemoveItem())))
	srv("/api/albums/save-image", quotaHandler(rateLimitHandler("act", handleAlbumSaveImage())))
	// 相册照片上传/访问
	srv("/api/albums/upload-photo", quotaHandler(rateLimitHandler("act", handleAlbumUploadPhoto())))
	// 论坛/文章正文图片上传 (V16.2)
	srv("/api/upload-image", quotaHandler(rateLimitHandler("act", handleUploadImage())))
	http.HandleFunc("/forum-image-file/", handleForumImageFile())
	http.HandleFunc("/album-photo-file/", handleAlbumPhotoFile())
	// 后台评价管理 (V7)
	srv("/api/admin/reviews", handleAdminListReviews(authStore))
	srv("/api/admin/reviews/", handleAdminReviewStatus(authStore))
	srv("/api/admin/review", handleAdminReview(authStore))
	srv("/api/admin/review/", handleAdminReview(authStore))
	// 后台帖子管理 (P1)
	srv("/api/admin/posts", handleAdminListPosts(authStore))
	srv("/api/admin/posts/", handleAdminListPosts(authStore))
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
	// 数据备份 (H5)
	srv("/api/admin/backup", handleAdminBackup())
	// AI 内容审查 (V16)
	srv("/api/admin/ai-review", handleAiReview())
	srv("/api/admin/ai-review/auto", handleAiReviewAuto())
	// 意见箱 (G0)
	srv("/api/opinion", quotaHandler(rateLimitHandler("act", handleSubmitOpinion(authStore))))
	// 拉黑 (G5)
	srv("/api/block", handleToggleBlock(authStore))
	srv("/api/block/list", handleBlockList(authStore))
	srv("/api/admin/opinions", handleAdminOpinions(authStore))
	srv("/api/admin/opinions/", handleAdminOpinions(authStore))

	// 工具中心：独立于 public/ 的构建包；仅已上线的公开工具可访问。
	registerToolRoutes(http.DefaultServeMux)

	// 静态前端 + 自定义 404（挂到 DefaultServeMux，与 srv() 注册的 /api 路由同源）
	// 目录由 baseDir 决定，不再受进程工作目录影响。
	http.Handle("/", newStaticHandler(publicDir()))
	// 维护模式中间件（普通访问者拦截，管理员/API 放行），包装 DefaultServeMux
	handler := securityHeaders(maintenanceInterceptor(authStore.db, sameOriginGuard(http.DefaultServeMux)))

	server, err := newHTTPServer(os.Getenv("PORT"), handler)
	if err != nil {
		log.Fatal("服务器配置无效: ", err)
	}
	cleanupCtx, stopCleanup := context.WithCancel(context.Background())
	defer stopCleanup()
	startRateLimitCleanup(cleanupCtx)
	log.Printf("🟣 QDU Wasteland 已启动: http://%s", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal("服务器启动失败: ", err)
	}
}
