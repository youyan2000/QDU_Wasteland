// 工具中心里的「热点日报」：七个方向 × 语种分栏，读本地快照渲染。
package hot

import (
	"net/http"
	"net/url"
	"time"

	"wasteland/internal/hotdigest"
	"wasteland/internal/tools"
)

// store 由主程序注入；nil 时该页 404。
var store *hotdigest.Store

// SetStore 注入数据源。
func SetStore(s *hotdigest.Store) { store = s }

func init() { tools.Register(tool{}) }

type tool struct{}

func (tool) Meta() tools.Meta {
	return tools.Meta{
		Slug:    "hot",
		Title:   "热点日报",
		Summary: "哲学理论、思想散文、当代诗歌、电力电子、嵌入式、AI、潮流音乐七个方向的每日推荐。",
		Label:   "SEVEN / DAILY",
		Access:  tools.Public,
		Enabled: true,
	}
}

var page = tools.ParsePage("hot.html")

type view struct {
	Date, GeneratedAt, Today  string
	SourceTotal, SourceFailed int
	Failed                    []string
	Tabs                      []tab
	Tracks                    []track
	Active                    tab
	ActiveTrack               track
	Items                     []hotdigest.Item
	PaperCount                int
	Days                      []string
	Empty                     bool
	CanRefresh                bool
}

type tab struct {
	Key    string
	Title  string
	Count  int
	Active bool
	Href   string
}

type track struct {
	Key    string
	Title  string
	Count  int
	Active bool
	Href   string
}

// tabHref 生成切换链接；非当天的日期要保留在 URL 里。
func tabHref(group, tr, day, today string) string {
	v := url.Values{}
	v.Set("group", group)
	if tr != "" {
		v.Set("track", tr)
	}
	if day != "" && day != today {
		v.Set("date", day)
	}
	return "/tool/hot/?" + v.Encode()
}

func (t tool) Register(mux *http.ServeMux, env tools.Env) {
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

		var d *hotdigest.Digest
		requestedDay := r.URL.Query().Get("date")
		if requestedDay != "" {
			if picked, err := store.Get(requestedDay); err == nil {
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
			v.SourceTotal, v.SourceFailed, v.Failed = d.SourceTotal, d.SourceFailed, d.Failed

			type listKey struct{ group, track string }
			byList := make(map[listKey][]hotdigest.Item, len(hotdigest.GroupOrder))
			groupCount := make(map[string]int, len(hotdigest.GroupOrder))
			for _, it := range d.Items {
				byList[listKey{it.Group, it.Track}] = append(byList[listKey{it.Group, it.Track}], it)
				groupCount[it.Group]++
			}

			// 展示顺序优先用 GroupOrder，新快照里新增的分组排在后面。
			order := append([]string{}, hotdigest.GroupOrder...)
			for g := range groupCount {
				known := false
				for _, k := range order {
					if k == g {
						known = true
						break
					}
				}
				if !known {
					order = append(order, g)
				}
			}

			active := r.URL.Query().Get("group")
			if active == "" || groupCount[active] == 0 {
				active = ""
				for _, g := range order {
					if groupCount[g] > 0 {
						active = g
						break
					}
				}
			}

			if hotdigest.IsSplit(active) {
				tr := r.URL.Query().Get("track")
				known := false
				for _, t := range hotdigest.TrackOrder {
					if t == tr {
						known = true
						break
					}
				}
				if !known {
					tr = ""
				}
				if tr == "" {
					best := -1
					for _, t := range hotdigest.TrackOrder {
						if n := len(byList[listKey{active, t}]); n > best {
							best, tr = n, t
						}
					}
				}
				for _, t := range hotdigest.TrackOrder {
					items := byList[listKey{active, t}]
					entry := track{Key: t, Title: hotdigest.TrackTitle(t), Count: len(items), Href: tabHref(active, t, d.Date, v.Today)}
					if t == tr {
						entry.Active = true
						v.ActiveTrack = entry
						v.Items = items
					}
					v.Tracks = append(v.Tracks, entry)
				}
			} else {
				v.Items = byList[listKey{active, ""}]
			}

			for _, g := range order {
				if groupCount[g] == 0 && g != active {
					continue
				}
				t := tab{Key: g, Title: hotdigest.GroupTitle(g), Count: groupCount[g], Href: tabHref(g, "", d.Date, v.Today)}
				if g == active {
					t.Active = true
					v.Active = t
				}
				v.Tabs = append(v.Tabs, t)
			}

			for _, it := range v.Items {
				if it.Kind == hotdigest.KindPaper {
					v.PaperCount++
				}
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
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodHead {
			return
		}
		_ = page.Execute(w, v)
	})
}
