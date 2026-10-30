// tools.go — 工具中心的装配处。
//
// 各工具是独立包，自己在 init() 里注册；这里只做三件事：
//  1. 读环境变量，构造日报数据源与静态包目录；
//  2. 把数据源注入对应工具；
//  3. 调 tools.Mount 一次，把清单页与全部工具挂到 /tool。
package main

import (
	"context"
	"html"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wasteland/internal/githubdigest"
	"wasteland/internal/hotdigest"
	"wasteland/internal/tools"
	_ "wasteland/internal/tools/collector"   // 随笔散文 + 趣味角落
	"wasteland/internal/tools/digest"        // 开源日报
		"wasteland/internal/tools/history"        // 浏览足迹
	"wasteland/internal/tools/hot"            // 热点日报
	_ "wasteland/internal/tools/nextjlc"      // Gerber 工具（托管外部静态包）
	_ "wasteland/internal/tools/placeholders" // history / favorite / poetrymatrix 占位
)

func registerToolRoutes(mux *http.ServeMux) {
	root := os.Getenv("TOOLS_ROOT")
	if root == "" {
		root = filepath.Join("dist", "tools")
	}
	ownerID, _ := strconv.ParseInt(os.Getenv("TOOL_OWNER_USER_ID"), 10, 64)
	canViewPrivate := func(r *http.Request) bool {
		// 本地演示开关：只对回环请求生效，且必须显式打开 TOOL_DEV_OWNER=1。
		// 生产环境不要设置；它不放宽任何公网请求。
		if os.Getenv("TOOL_DEV_OWNER") == "1" && isLoopbackPeer(r) {
			return true
		}
		uid := currentUserID(r)
		if ownerID <= 0 || uid != ownerID {
			return false
		}
		u, err := authStore.GetByID(uid)
		return err == nil && u != nil && u.Banned == 0
	}

	// 浏览足迹：与站点共用同一个 SQLite 库（表名前缀 history_）。
	if st, err := history.NewStore(authStore.db); err != nil {
		log.Printf("⚠️ history 工具初始化失败: %v", err)
	} else {
		history.Setup(st)
	}
	// 每日硬件开源项目摘要：数据落本地 JSON（默认 ./tool-data），不写站点数据库。
	// GITHUB_TOKEN 可选，用于把搜索 API 配额从 10 次/分钟提高到 30 次/分钟。
	digestDir := os.Getenv("TOOLS_DATA_DIR")
	if digestDir == "" {
		digestDir = "tool-data"
	}
	digests := githubdigest.NewStore(digestDir, strings.TrimSpace(os.Getenv("GITHUB_TOKEN")))
	digests.DB = authStore.db // 快照入单库（表 digest_issues），不再散落 JSON 文件
	digests.Delay = 1500 * time.Millisecond
	digests.RunDaily(context.Background(), notifyDigest)

	// 每日热点：哲学理论 / 思想散文 / 当代诗歌 / 电力电子 / 嵌入式 / AI / 潮流音乐。
	// AI_API_KEY 可选：配置后每个方向各一次模型调用；未配置时全部走规则。
	hots := hotdigest.NewStore(digestDir)
	hots.DB = authStore.db // 表 hot_issues
	if strings.TrimSpace(os.Getenv("HOT_SUMMARY")) != "off" {
		hots.Enrich = hotdigest.NewEnricher(strings.TrimSpace(os.Getenv("AI_API_KEY")))
	}
	hots.RunDaily(context.Background(), notifyHot)

	digest.SetStore(digests)
	hot.SetStore(hots)

	tools.Mount(mux, tools.Env{
		DataDir:    digestDir,
		BundleRoot: filepath.Join(root, "nextjlc"),
		OwnerUID:   ownerID,
		CanOwner:   canViewPrivate,
	})

	// 仅站长可用的手动刷新；页面按钮调用此接口。
	srv("/api/tools/digest/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			apiErr(w, http.StatusMethodNotAllowed, "只支持 POST")
			return
		}
		if !canViewPrivate(r) {
			apiErr(w, http.StatusNotFound, "未找到")
			return
		}
		d, err := digests.Refresh(r.Context(), true)
		if err != nil {
			apiErr(w, http.StatusBadGateway, "抓取失败："+err.Error())
			return
		}
		apiJSON(w, http.StatusOK, map[string]any{"ok": true, "date": d.Date, "count": len(d.Items)})
	})

	// 热点日报同样只有站长能触发抓取；页面本身对访客公开。
	srv("/api/tools/hot/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			apiErr(w, http.StatusMethodNotAllowed, "只支持 POST")
			return
		}
		if !canViewPrivate(r) {
			apiErr(w, http.StatusNotFound, "未找到")
			return
		}
		d, err := hots.Refresh(r.Context(), true)
		if err != nil {
			apiErr(w, http.StatusBadGateway, "抓取失败："+err.Error())
			return
		}
		apiJSON(w, http.StatusOK, map[string]any{"ok": true, "date": d.Date, "count": len(d.Items)})
	})
}

// notifyDigest 把新生成的日报邮件推送给站长；未配置 SMTP 时静默跳过。
func notifyDigest(d *githubdigest.Digest) {
	if d == nil || len(d.Items) == 0 {
		return
	}
	if strings.TrimSpace(os.Getenv("MAIL_HOST")) == "" {
		return
	}
	to := ownerMailbox()
	if to == "" {
		return
	}

	var body strings.Builder
	body.WriteString(`<h3>开源日报 ` + html.EscapeString(d.Date) + `</h3>`)
	byGroup := make(map[string][]githubdigest.Item, len(githubdigest.GroupOrder))
	for _, it := range d.Items {
		byGroup[it.Group] = append(byGroup[it.Group], it)
	}
	for _, g := range githubdigest.GroupOrder {
		items := byGroup[g]
		if len(items) == 0 {
			continue
		}
		body.WriteString(`<h4 style="margin:18px 0 6px">` + html.EscapeString(githubdigest.GroupTitle(g)) + `</h4><ol>`)
		for _, it := range items {
			body.WriteString(`<li><a href="` + html.EscapeString(it.URL) + `">` +
				html.EscapeString(it.FullName) + `</a>　★` + strconv.Itoa(it.Stars) +
				`<br><span style="color:#666">` + html.EscapeString(it.Description) + `</span></li>`)
		}
		body.WriteString(`</ol>`)
	}
	body.WriteString(`<p>共 ` + strconv.Itoa(len(d.Items)) + ` 个项目。</p>`)
	sendMail(to, "开源日报 "+d.Date+"（"+strconv.Itoa(len(d.Items))+" 个项目）", body.String())
}

// notifyHot 把新生成的热点日报推送给站长；未配置 SMTP 时静默跳过。
func notifyHot(d *hotdigest.Digest) {
	if d == nil || len(d.Items) == 0 {
		return
	}
	if strings.TrimSpace(os.Getenv("MAIL_HOST")) == "" {
		return
	}
	to := ownerMailbox()
	if to == "" {
		return
	}

	var body strings.Builder
	body.WriteString(`<h3>热点日报 ` + html.EscapeString(d.Date) + `</h3>`)
	byGroup := make(map[string][]hotdigest.Item, len(hotdigest.GroupOrder))
	for _, it := range d.Items {
		byGroup[it.Group] = append(byGroup[it.Group], it)
	}
	for _, g := range hotdigest.GroupOrder {
		items := byGroup[g]
		if len(items) == 0 {
			continue
		}
		body.WriteString(`<h4 style="margin:18px 0 6px">` + html.EscapeString(hotdigest.GroupTitle(g)) + `</h4><ol>`)
		for _, it := range items {
			body.WriteString(`<li><a href="` + html.EscapeString(it.URL) + `">` +
				html.EscapeString(it.Title) + `</a>　<span style="color:#888">` +
				html.EscapeString(it.Source) + `</span>`)
			if it.Summary != "" {
				body.WriteString(`<br><span style="color:#666">` + html.EscapeString(it.Summary) + `</span>`)
			}
			body.WriteString(`</li>`)
		}
		body.WriteString(`</ol>`)
	}
	body.WriteString(`<p>共 ` + strconv.Itoa(len(d.Items)) + ` 条，汇总 ` +
		strconv.Itoa(d.SourceTotal-d.SourceFailed) + ` 个信源。</p>`)
	sendMail(to, "热点日报 "+d.Date+"（"+strconv.Itoa(len(d.Items))+" 条）", body.String())
}

// ownerMailbox 解析日报收件人：优先 TOOL_OWNER_USER_ID 对应用户的邮箱，回退 ADMIN_EMAIL。
func ownerMailbox() string {
	if uid, _ := strconv.ParseInt(os.Getenv("TOOL_OWNER_USER_ID"), 10, 64); uid > 0 && authStore != nil {
		if u, err := authStore.GetByID(uid); err == nil && u != nil {
			if email := strings.TrimSpace(u.Email); email != "" {
				return email
			}
		}
	}
	return strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))
}

// isLoopbackPeer 判断请求是否来自本机（用于仅本地生效的演示开关）。
func isLoopbackPeer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}