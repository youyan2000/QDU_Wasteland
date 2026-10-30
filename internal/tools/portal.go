package tools

import (
	"bytes"
	"net/http"
)

// portalPage 沿用主站既有的工具中心模板（static/index.html），外观与线上保持一致。
var portalPage = ParsePage("index.html")

type portalView struct {
	Tools []Meta
	Demo  bool
}

// renderPortal 渲染清单页。
func renderPortal(w http.ResponseWriter, r *http.Request, list []Meta) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method == http.MethodHead {
		return
	}
	_ = portalPage.Execute(w, portalView{Tools: list})
}

// renderPortalString 供测试使用。
func renderPortalString(list []Meta) string {
	var buf bytes.Buffer
	_ = portalPage.Execute(&buf, portalView{Tools: list})
	return buf.String()
}
