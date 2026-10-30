// 工具中心里的两个「收藏导航」页：随笔散文（information）与趣味角落（toy）。
// 两者共用同一套页面与数据文件，只是取不同的书签分组。
package collector

import (
	"encoding/json"
	"html/template"
	"net/http"

	"wasteland/internal/tools"
)

func init() {
	tools.Register(page{slug: "information", title: "随笔散文",
		summary: "杂志、文学、哲学、科学与技术：留一些值得慢慢读的地方。",
		label:   "READ / THINK", collection: "magazine"})
	tools.Register(page{slug: "toy", title: "趣味角落",
		summary: "一些好玩、古怪、可以点开探索的小网站。",
		label:   "JUST FOR FUN", collection: "toy"})
}

// Bookmark 是一个外部站点条目。
type Bookmark struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Cover       string `json:"cover"`
	Image       string `json:"image"`
}

type collections struct {
	Magazine []Bookmark `json:"magazine"`
	Toy      []Bookmark `json:"toy"`
}

// bookmarks 在包初始化时从 embed 里读一次。
var bookmarks = func() collections {
	data, err := tools.AssetsFS().Open("curated-bookmarks.json")
	if err != nil {
		return collections{}
	}
	defer data.Close()
	var out collections
	if err := json.NewDecoder(data).Decode(&out); err != nil {
		return collections{}
	}
	return out
}()

var tmpl = tools.ParsePage("collection.html")

type page struct {
	slug, title, summary, label, collection string
}

func (p page) Meta() tools.Meta {
	return tools.Meta{
		Slug: p.slug, Title: p.title, Summary: p.summary, Label: p.label,
		Access: tools.Public, Enabled: true,
	}
}

func (p page) Register(mux *http.ServeMux, _ tools.Env) {
	items := bookmarks.Magazine
	intro := "杂志、文学、哲学与科学：留一些值得慢慢读的地方。"
	if p.collection == "toy" {
		items = bookmarks.Toy
		intro = "一些好玩、古怪、可以点开探索的小网站。"
	}
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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodHead {
			return
		}
		_ = tmpl.Execute(w, struct {
			Title, Intro, Slug string
			Items              []Bookmark
		}{p.title, intro, p.slug, items})
	})
}

var _ = template.Must
