package tools

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// mount 是注册表在路由层的投影：元信息 + 该工具自己的子 mux。
type mount struct {
	meta Meta
	sub  *http.ServeMux
}

// dispatcher 是 /tool 与 /tool/ 的唯一入口：
// 它负责路径安全、权限判定、清单页渲染，然后把请求交给对应工具的子 mux。
type dispatcher struct {
	env    Env
	order  []string
	mounts map[string]mount
}

func newDispatcher(env Env) *dispatcher {
	d := &dispatcher{env: env, mounts: make(map[string]mount)}
	for _, t := range All() {
		m := t.Meta()
		sub := http.NewServeMux()
		t.Register(sub, env)
		d.mounts[m.Slug] = mount{meta: m, sub: sub}
		d.order = append(d.order, m.Slug)
	}
	return d
}

func (d *dispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if !strings.HasPrefix(path, "/tool") {
		http.NotFound(w, r)
		return
	}
	if !validToolPath(path) {
		http.NotFound(w, r)
		return
	}

	switch {
	case path == "/tool":
		// 统一带斜杠，避免相对链接在 /tool 下解析错误。
		target := "/tool/"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
		return
	case path == "/tool/":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		renderPortal(w, r, d.visible(r))
		return
	case strings.HasPrefix(path, "/tool/_assets/"):
		// 各工具共用的 CSS / JS / 封面图。
		serveAsset(w, r)
		return
	}

	rest := strings.TrimPrefix(path, "/tool/")
	slug, tail, hadSlash := strings.Cut(rest, "/")
	entry, ok := d.mounts[slug]
	if !ok || !entry.meta.Enabled {
		http.NotFound(w, r)
		return
	}
	if entry.meta.Access == Member && d.env.userID(r) <= 0 {
		denyMember(w, r)
		return
	}
	if !d.allowed(r, entry.meta) {
		// OwnerOnly 对非站长一律 404，不泄露工具是否存在。
		http.NotFound(w, r)
		return
	}
	if !hadSlash {
		// 只有真的缺尾斜杠时才补：否则 /tool/x/ 会重定向给自己，变成死循环。
		target := "/tool/" + slug + "/"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
		return
	}

	// 交给工具自己的子 mux：它看到的是 /history/... 去掉 slug 之后的路径。
	sub := r.Clone(r.Context())
	sub.URL.Path = "/" + tail
	entry.sub.ServeHTTP(w, sub)
}

// allowed 判定当前请求能否访问该工具。
func (d *dispatcher) allowed(r *http.Request, m Meta) bool {
	switch m.Access {
	case OwnerOnly:
		return d.env.isOwner(r)
	case Member:
		return d.env.userID(r) > 0
	default:
		return true
	}
}

func denyMember(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/tool/") && strings.Contains(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "请先登录"})
		return
	}
	next := r.URL.RequestURI()
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/tool/"
	}
	http.Redirect(w, r, "/login.html?next="+url.QueryEscape(next), http.StatusFound)
}

// visible 给出当前访客在清单页上应该看到的条目。
func (d *dispatcher) visible(r *http.Request) []Meta {
	out := make([]Meta, 0, len(d.order))
	for _, slug := range d.order {
		m := d.mounts[slug].meta
		if m.Access == Member && d.env.userID(r) <= 0 {
			continue
		}
		if !d.allowed(r, m) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// validToolPath 挡住编码绕过、反斜杠、点段与空段。
func validToolPath(p string) bool {
	if strings.ContainsAny(p, "\\%") {
		return false
	}
	rest := strings.TrimPrefix(p, "/tool")
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return true
	}
	for _, seg := range strings.Split(rest, "/") {
		if strings.HasPrefix(seg, ".") {
			return false
		}
	}
	return true
}
