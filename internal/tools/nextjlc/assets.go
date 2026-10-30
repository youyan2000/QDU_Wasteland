// assets.go — 前端资源（原生 HTML/JS，无构建）。
package nextjlc

import (
	"embed"
	"io/fs"
)

//go:embed static
var webAssets embed.FS

// webFS 返回 static/ 子树。
func webFS() fs.FS {
	sub, err := fs.Sub(webAssets, "static")
	if err != nil {
		panic("nextjlc: 静态资源缺失: " + err.Error())
	}
	return sub
}