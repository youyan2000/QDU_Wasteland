// 命令 toolsdemo 是工具框架的本地演示：不依赖主站，单独起一个进程，
// 用来说明「注册表 → 清单页 → 路由分发 → 权限」这条链路。
//
// 用法：
//
//	go run ./cmd/toolsdemo -addr 127.0.0.1:3100
//	浏览器打开 http://127.0.0.1:3100/tool/
//	点「以站长身份进入」后再看私有工具（history）
package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"wasteland/internal/tools"
)

//go:embed static
var assets embed.FS

// demoTool 把「元信息 + 一个 handler」包成 Tool。
type demoTool struct {
	meta tools.Meta
	h    http.Handler
}

func (d demoTool) Meta() tools.Meta { return d.meta }

func (d demoTool) Register(mux *http.ServeMux, _ tools.Env) {
	if d.h != nil {
		mux.Handle("/", d.h)
	}
}

// page 生成一个极简 HTML 页面。
func page(title, body string) http.Handler {
	html := `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>` + title + `</title><style>` +
		`body{margin:0;background:#faf9f6;color:#16181d;font:16px/1.7 system-ui,"Microsoft YaHei",sans-serif}` +
		`.w{max-width:760px;margin:0 auto;padding:48px 24px}a{color:#304cff}` +
		`h1{font-size:28px;margin:0 0 16px}.tag{font:11px/1 ui-monospace,monospace;letter-spacing:.1em;color:#6b7280}` +
		`</style></head><body><div class="w"><p class="tag">` + title + `</p><h1>` + title + `</h1>` + body +
		`<p style="margin-top:32px"><a href="/tool/">← 回到工具中心</a></p></div></body></html>`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write([]byte(html))
	})
}

func main() {
	addr := flag.String("addr", "127.0.0.1:3100", "监听地址")
	flag.Parse()

	nextjlc, _ := fs.Sub(assets, "static/nextjlc")
	history, _ := fs.Sub(assets, "static/history")

	// 8 个工具位：5 个可用 + 3 个占位，顺序由框架的 displayOrder 决定。
	tools.Register(demoTool{
		meta: tools.Meta{Slug: "nextjlc", Title: "NeXTJLC", Summary: "在线整理 Gerber 文件并导出 JLC 格式。",
			Label: "PCB / GERBER", Access: tools.Public, Enabled: true},
		h: tools.Static{FS: nextjlc},
	})
	tools.Register(demoTool{
		meta: tools.Meta{Slug: "information", Title: "随笔散文", Summary: "杂志、文学、哲学与科学：留一些值得慢慢读的地方。",
			Label: "READ / THINK", Access: tools.Public, Enabled: true},
		h: page("随笔散文", `<p>书签导航占位页。真实版本会从 <code>data/</code> 读取书签 JSON 与封面图。</p>`),
	})
	tools.Register(demoTool{
		meta: tools.Meta{Slug: "toy", Title: "趣味角落", Summary: "一些好玩、古怪、可以点开探索的小网站。",
			Label: "JUST FOR FUN", Access: tools.Public, Enabled: true},
		h: page("趣味角落", `<p>书签导航占位页。</p>`),
	})
	tools.Register(demoTool{
		meta: tools.Meta{Slug: "digest", Title: "开源日报", Summary: "每日 GitHub 热门开源项目，分硬件、AI、音乐三组。",
			Label: "GITHUB / DAILY", Access: tools.Public, Enabled: true},
		h: page("开源日报", `<p>日报占位页。真实版本从 SQLite 读取当日快照。</p>`),
	})
	tools.Register(demoTool{
		meta: tools.Meta{Slug: "hot", Title: "热点日报", Summary: "哲学、散文、诗歌、电力电子、嵌入式、AI、音乐七个方向的每日推荐。",
			Label: "HOT / DAILY", Access: tools.Public, Enabled: true},
		h: page("热点日报", `<p>热点日报占位页。真实版本从 SQLite 读取当日快照。</p>`),
	})
	tools.Register(demoTool{
		meta: tools.Meta{Slug: "history", Title: "浏览足迹", Summary: "浏览器历史、B站历史与书签，仅本人可见。",
			Label: "PRIVATE / HISTORY", Access: tools.OwnerOnly, Enabled: true},
		h: tools.Static{FS: history},
	})
	tools.Register(demoTool{
		meta: tools.Meta{Slug: "poetrymatrix", Title: "PoetryMatrix", Summary: "为诗意留一块地方。独立工具准备好后从这里接入。",
			Label: "POETRY", Access: tools.Public, Enabled: false},
	})
	tools.Register(demoTool{
		meta: tools.Meta{Slug: "favorite", Title: "私人收藏", Summary: "仅本人可见的网址收藏，与公开导航分开存放。",
			Label: "PRIVATE / BOOKMARKS", Access: tools.OwnerOnly, Enabled: false},
	})

	mux := http.NewServeMux()

	// 演示用的「以站长身份进入」：设置 cookie 后回到清单页。
	// 真实站点这里换成站点会话判定。
	mux.HandleFunc("/demo-owner", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("off") {
		case "1":
			http.SetCookie(w, &http.Cookie{Name: "demo_owner", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		default:
			http.SetCookie(w, &http.Cookie{Name: "demo_owner", Value: "1", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		}
		http.Redirect(w, r, "/tool/", http.StatusSeeOther)
	})

	tools.Mount(mux, tools.Env{
		DataDir:  "data",
		OwnerUID: 1,
		CanOwner: func(r *http.Request) bool {
			c, err := r.Cookie("demo_owner")
			return err == nil && c.Value == "1"
		},
	})

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("工具框架演示: http://%s/tool/", *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
		os.Exit(1)
	}
}
