// 工具中心里的「开源日报」：读本地快照，按主题分段渲染。
package digest

import (
	"html/template"
	"net/http"
	"sort"
	"time"

	"wasteland/internal/githubdigest"
	"wasteland/internal/tools"
)

// store 由主程序在启动时注入（nil 时该页 404，等价于未启用）。
var store *githubdigest.Store

// SetStore 注入日报数据源。
func SetStore(s *githubdigest.Store) { store = s }

func init() { tools.Register(tool{}) }

type tool struct{}

func (tool) Meta() tools.Meta {
	return tools.Meta{
		Slug:    "digest",
		Title:   "开源日报",
		Summary: "每日 GitHub 热门开源项目，分硬件、AI、音乐三组。",
		Label:   "GITHUB / DAILY",
		Access:  tools.Public,
		Enabled: true,
	}
}

var page = tools.ParsePage("digest.html")

type view struct {
	Date, GeneratedAt, Today string
	Sections                 []section
	Days                     []string
	Empty                    bool
	CanRefresh               bool
}

type section struct {
	Key   string
	Title string
	Items []githubdigest.Item
}

// whenLabel 把发布时间渲染成"几分钟前"。
func whenLabel(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < 0:
		return t.Format("01-02 15:04")
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return time.Duration(d).Round(time.Minute).String()
	case d < 24*time.Hour:
		return time.Duration(d).Round(time.Hour).String()
	case d < 48*time.Hour:
		return "昨天 " + t.Format("15:04")
	default:
		return t.Format("01-02")
	}
}

func (t tool) Register(mux *http.ServeMux, env tools.Env) {
	_ = template.Must
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if store == nil {
			http.NotFound(w, r)
			return
		}

		v := view{
			Today:      time.Now().Format("2006-01-02"),
			CanRefresh: env.CanOwner != nil && env.CanOwner(r),
		}

		var d *githubdigest.Digest
		if day := r.URL.Query().Get("date"); day != "" {
			if picked, err := store.Get(day); err == nil {
				d = picked
			}
		}
		if d == nil {
			if latest, err := store.Latest(); err == nil {
				d = latest
			}
		}

		if d == nil || len(d.Items) == 0 {
			v.Empty = true
		} else {
			v.Date, v.GeneratedAt = d.Date, d.GeneratedAt
			byGroup := make(map[string][]githubdigest.Item, len(githubdigest.GroupOrder))
			for _, it := range d.Items {
				byGroup[it.Group] = append(byGroup[it.Group], it)
			}
			for _, g := range githubdigest.GroupOrder {
				if len(byGroup[g]) == 0 {
					continue
				}
				v.Sections = append(v.Sections, section{Key: g, Title: githubdigest.GroupTitle(g), Items: byGroup[g]})
				delete(byGroup, g)
			}
			// 新快照里出现的分组也不能丢。
			rest := make([]string, 0, len(byGroup))
			for g := range byGroup {
				rest = append(rest, g)
			}
			sort.Strings(rest)
			for _, g := range rest {
				v.Sections = append(v.Sections, section{Key: g, Title: githubdigest.GroupTitle(g), Items: byGroup[g]})
			}
		}

		if days, err := store.Days(); err == nil {
			if len(days) > 10 {
				days = days[:10]
			}
			v.Days = days
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodHead {
			return
		}
		_ = page.Execute(w, v)
	})
}
