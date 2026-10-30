// 尚未实现的工具位。
//
// 它们以 Enabled=false 进注册表：清单页照常显示"准备中"，但不挂任何路由，
// 因此 /tool/history/ 这类地址返回 404，不泄露存在性。
package placeholders

import (
	"net/http"

	"wasteland/internal/tools"
)

func init() {

	tools.Register(entry{meta: tools.Meta{
		Slug: "favorite", Title: "私人收藏",
		Summary: "仅本人可见的网址收藏，与公开导航分开存放。",
		Label:   "PRIVATE / BOOKMARKS",
		Access:  tools.OwnerOnly, Enabled: false,
	}})
	tools.Register(entry{meta: tools.Meta{
		Slug: "poetrymatrix", Title: "PoetryMatrix",
		Summary: "为诗意留一块地方。独立工具准备好后从这里接入。",
		Label:   "POETRY",
		Access:  tools.Public, Enabled: false,
	}})
}

type entry struct{ meta tools.Meta }

func (e entry) Meta() tools.Meta { return e.meta }

// Register 故意留空：未启用的工具没有路由。
func (e entry) Register(*http.ServeMux, tools.Env) {}
