package main

import (
	"context"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"qdu-wasteland/internal/githubdigest"
	"qdu-wasteland/internal/toolweb"
)

func registerToolRoutes(mux *http.ServeMux) {
	root := os.Getenv("TOOLS_ROOT")
	if root == "" {
		root = filepath.Join("dist", "tools")
	}
	ownerID, _ := strconv.ParseInt(os.Getenv("TOOL_OWNER_USER_ID"), 10, 64)
	canViewPrivate := func(r *http.Request) bool {
		uid := currentUserID(r)
		if ownerID <= 0 || uid != ownerID {
			return false
		}
		u, err := authStore.GetByID(uid)
		return err == nil && u != nil && u.Banned == 0
	}

	// 每日硬件开源项目摘要：数据落本地 JSON（默认 ./tool-data），不写站点数据库。
	// GITHUB_TOKEN 可选，用于把搜索 API 配额从 10 次/分钟提高到 30 次/分钟。
	digestDir := os.Getenv("TOOLS_DATA_DIR")
	if digestDir == "" {
		digestDir = "tool-data"
	}
	digests := githubdigest.NewStore(digestDir, strings.TrimSpace(os.Getenv("GITHUB_TOKEN")))
	digests.Delay = 1500 * time.Millisecond
	digests.RunDaily(context.Background(), notifyDigest)

	handler := toolweb.New(toolweb.Options{
		Root:           root,
		CanViewPrivate: canViewPrivate,
		Digest:         digests,
	})
	mux.Handle("/tool", handler)
	mux.Handle("/tool/", handler)

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
}

// notifyDigest 把新生成的日报邮件推送给站长；未配置 SMTP 时静默跳过。
func notifyDigest(d *githubdigest.Digest) {
	if d == nil || len(d.Items) == 0 {
		return
	}
	to := ""
	if uid, _ := strconv.ParseInt(os.Getenv("TOOL_OWNER_USER_ID"), 10, 64); uid > 0 && authStore != nil {
		if u, err := authStore.GetByID(uid); err == nil && u != nil {
			to = strings.TrimSpace(u.Email)
		}
	}
	if to == "" {
		to = strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))
	}
	if to == "" {
		return
	}

	var body strings.Builder
	body.WriteString(`<h3>开源日报 ` + html.EscapeString(d.Date) + `</h3><ol>`)
	for _, it := range d.Items {
		body.WriteString(`<li><a href="` + html.EscapeString(it.URL) + `">` +
			html.EscapeString(it.FullName) + `</a>　★` + strconv.Itoa(it.Stars) +
			`<br><span style="color:#666">` + html.EscapeString(it.Description) + `</span></li>`)
	}
	body.WriteString(`</ol><p>共 ` + strconv.Itoa(len(d.Items)) + ` 个项目。</p>`)
	sendMail(to, "开源日报 "+d.Date+"（"+strconv.Itoa(len(d.Items))+" 个项目）", body.String())
}
