// 页面渲染冒烟测试：模板字段与视图结构对不上时，Execute 会返回错误，
// 而线上只表现为"页面空白"，很难察觉。这里把每个工具页都渲染一遍。
package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPortalTemplateRenders(t *testing.T) {
	html := renderPortalString([]Meta{
		{Slug: "alpha", Title: "Alpha", Summary: "简介", Label: "L", Enabled: true},
		{Slug: "beta", Title: "Beta", Summary: "简介", Label: "L", Enabled: false},
	})
	if !strings.Contains(html, "工具") || !strings.Contains(html, "/tool/alpha/") {
		t.Fatalf("清单页渲染异常: %.160s", html)
	}
	if strings.Contains(html, `href="/tool/beta/"`) {
		t.Error("未启用工具不应有链接")
	}
}

func TestCollectionTemplateRenders(t *testing.T) {
	tmpl := ParsePage("collection.html")
	var sb strings.Builder
	err := tmpl.Execute(&sb, map[string]any{
		"Title": "随笔散文", "Intro": "简介", "Slug": "information",
		"Items": []any{},
	})
	if err != nil {
		t.Fatalf("collection.html 渲染失败: %v", err)
	}
	if sb.Len() == 0 {
		t.Fatal("collection.html 渲染为空")
	}
}

func TestFeedTemplatesRender(t *testing.T) {
	// digest.html / hot.html 的视图由各工具包提供；用空 map 渲染可验证
	// "模板语法 + 函数注册"没问题（空 map 的缺失字段取零值，不会报错）。
	for _, name := range []string{"digest.html", "hot.html"} {
		tmpl := ParsePage(name)
		var sb strings.Builder
		if err := tmpl.Execute(&sb, map[string]any{
			"Items": []any{}, "Tabs": []any{}, "Tracks": []any{}, "Days": []any{},
		}); err != nil {
			t.Fatalf("%s 渲染失败: %v", name, err)
		}
		if !strings.Contains(sb.String(), "<html") {
			t.Fatalf("%s 输出不像 HTML", name)
		}
	}
}

func TestCuratedBookmarksParse(t *testing.T) {
	f, err := AssetsFS().Open("curated-bookmarks.json")
	if err != nil {
		t.Fatalf("读不到书签数据: %v", err)
	}
	defer f.Close()
	var v struct {
		Magazine []map[string]any `json:"magazine"`
		Toy      []map[string]any `json:"toy"`
	}
	if err := json.NewDecoder(f).Decode(&v); err != nil {
		t.Fatalf("书签 JSON 解析失败: %v", err)
	}
	if len(v.Magazine) == 0 || len(v.Toy) == 0 {
		t.Fatalf("书签分组为空: magazine=%d toy=%d", len(v.Magazine), len(v.Toy))
	}
}
