package tools

import (
	"fmt"
	"net/http"
	"regexp"
	"sync"
)

// displayOrder 固定清单页的展示顺序：先自有工具，再日报，最后是历史类与占位。
// 未列出的 slug 按注册顺序排在末尾，这样新增工具不会悄悄改变已有卡片的位置。
var displayOrder = []string{
	"nextjlc",
	"information",
	"toy",
	"digest",
	"hot",
	"history",
	"favorite",
	"poetrymatrix",
}

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

var (
	registryMu sync.Mutex
	registry   []Tool
)

// Register 由各工具包的 init() 调用。违反约定时直接 panic：
// 这类错误只会在启动瞬间出现，静默降级反而会变成线上 404。
func Register(t Tool) {
	if t == nil {
		panic("tools: Register(nil)")
	}
	m := t.Meta()
	if !slugPattern.MatchString(m.Slug) {
		panic(fmt.Sprintf("tools: invalid slug %q (want ^[a-z][a-z0-9-]{0,31}$)", m.Slug))
	}
	if m.Title == "" {
		panic(fmt.Sprintf("tools: tool %q has empty Title", m.Slug))
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	for _, existing := range registry {
		if existing.Meta().Slug == m.Slug {
			panic(fmt.Sprintf("tools: duplicate slug %q", m.Slug))
		}
	}
	registry = append(registry, t)
}

// All 返回按展示顺序排好的工具快照。调用方可以自由修改返回值。
func All() []Tool {
	registryMu.Lock()
	defer registryMu.Unlock()
	out := make([]Tool, len(registry))
	copy(out, registry)
	return sortByDisplayOrder(out)
}

// sortByDisplayOrder 按 displayOrder 排序，未列出的保持原有相对顺序。
func sortByDisplayOrder(list []Tool) []Tool {
	rank := make(map[string]int, len(displayOrder))
	for i, slug := range displayOrder {
		rank[slug] = i
	}
	pos := func(t Tool) int {
		if r, ok := rank[t.Meta().Slug]; ok {
			return r
		}
		return len(displayOrder)
	}
	// 稳定插入排序：列表规模很小（个位数），可读性优先。
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && pos(list[j]) < pos(list[j-1]); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
	return list
}

// Mount 把工具清单页与所有已启用工具挂到 mux 上。
// 主程序只需要调用这一次。
func Mount(mux *http.ServeMux, env Env) {
	d := newDispatcher(env)
	if mux == nil {
		panic("tools: Mount(nil mux)")
	}
	mux.Handle("/tool", d)
	mux.Handle("/tool/", d)
}

// resetForTest 清空注册表，仅供测试使用。
func resetForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = nil
}
