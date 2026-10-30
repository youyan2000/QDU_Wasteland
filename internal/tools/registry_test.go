package tools

import (
	"net/http"
	"testing"
)

// fakeTool 是测试用的最小工具实现。
type fakeTool struct {
	meta     Meta
	register func(*http.ServeMux, Env)
}

func (f *fakeTool) Meta() Meta { return f.meta }

func (f *fakeTool) Register(mux *http.ServeMux, env Env) {
	if f.register != nil {
		f.register(mux, env)
	}
}

func TestRegisterRejectsInvalidSlug(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	for _, slug := range []string{"", "History", "his tory", "-bad", "a/b", "有中文"} {
		slug := slug
		t.Run(slug, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("slug %q 应当 panic", slug)
				}
			}()
			Register(&fakeTool{meta: Meta{Slug: slug, Title: "x"}})
		})
	}
}

func TestRegisterRejectsEmptyTitle(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	defer func() {
		if recover() == nil {
			t.Fatal("空 Title 应当 panic")
		}
	}()
	Register(&fakeTool{meta: Meta{Slug: "ok", Title: ""}})
}

func TestRegisterRejectsDuplicateSlug(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	Register(&fakeTool{meta: Meta{Slug: "dup", Title: "one"}})
	defer func() {
		if recover() == nil {
			t.Fatal("重复 slug 应当 panic")
		}
	}()
	Register(&fakeTool{meta: Meta{Slug: "dup", Title: "two"}})
}

func TestAllFollowsDisplayOrder(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	// 故意乱序注册。
	for _, slug := range []string{"history", "hot", "nextjlc", "custom", "digest"} {
		Register(&fakeTool{meta: Meta{Slug: slug, Title: slug}})
	}

	got := make([]string, 0, 5)
	for _, tool := range All() {
		got = append(got, tool.Meta().Slug)
	}

	want := []string{"nextjlc", "digest", "hot", "history", "custom"}
	if len(got) != len(want) {
		t.Fatalf("数量不符: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("顺序不符: got=%v want=%v", got, want)
		}
	}
}

func TestAllReturnsCopy(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	Register(&fakeTool{meta: Meta{Slug: "only", Title: "only"}})
	list := All()
	list[0] = nil

	if All()[0] == nil {
		t.Fatal("All() 必须返回副本，外部修改不应影响注册表")
	}
}
