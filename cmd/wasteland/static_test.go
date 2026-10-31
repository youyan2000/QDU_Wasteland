// static_test.go — 静态路由的行为契约。
//
// 用 httptest + 临时目录验证，不依赖真起服务器（本机杀软会吃掉新编译的 exe，
// 端到端验证不可靠，所以把"能返回什么"钉在单测里）。
package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestSite 造一个带 index/404/静态文件的假站点。
func newTestSite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("建目录失败: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("写 %s 失败: %v", name, err)
		}
	}
	write("index.html", "<html>HOME</html>")
	write("courses.html", "<html>COURSES</html>")
	write("style.css", "body{color:#000}")
	write("404.html", "<html>PAGE_NOT_FOUND</html>")
	write(filepath.Join("sub", "index.html"), "<html>SUB</html>")
	return dir
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestStaticServesIndex(t *testing.T) {
	h := newStaticHandler(newTestSite(t))
	rec := get(t, h, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("/ 状态码 = %d，期望 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "HOME") {
		t.Fatalf("/ 返回内容不是 index.html: %q", rec.Body.String())
	}
}

func TestStaticServesFile(t *testing.T) {
	h := newStaticHandler(newTestSite(t))
	for _, tc := range []struct{ path, want string }{
		{"/courses.html", "COURSES"},
		{"/style.css", "color:#000"},
	} {
		rec := get(t, h, tc.path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s 状态码 = %d，期望 200", tc.path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s 内容不符: %q", tc.path, rec.Body.String())
		}
	}
}

func TestStaticServesSubdirIndex(t *testing.T) {
	h := newStaticHandler(newTestSite(t))
	rec := get(t, h, "/sub/")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "SUB") {
		t.Fatalf("/sub/ = %d %q", rec.Code, rec.Body.String())
	}
}

// 关键回归：缺页必须是 404，不能是 200（旧实现用 ServeFile 输出 404.html 会给 200）。
func TestStaticMissingIs404WithBrandPage(t *testing.T) {
	h := newStaticHandler(newTestSite(t))
	rec := get(t, h, "/nope.html")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("缺页状态码 = %d，期望 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "PAGE_NOT_FOUND") {
		t.Fatalf("缺页没有返回品牌 404 页: %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q", ct)
	}
}

// 目录存在但没有 index.html 时也走 404 页，而不是列目录。
func TestStaticDirWithoutIndexIs404(t *testing.T) {
	dir := newTestSite(t)
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := get(t, newStaticHandler(dir), "/empty/")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404", rec.Code)
	}
}

// 连 404.html 都没有时退回纯文本 404，表明"目录没找对"。
func TestStaticMissing404PageFallsBack(t *testing.T) {
	dir := t.TempDir()
	rec := get(t, newStaticHandler(dir), "/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404", rec.Code)
	}
}

// 路径遍历一律拒绝。
func TestStaticRejectsTraversal(t *testing.T) {
	h := newStaticHandler(newTestSite(t))
	for _, p := range []string{"/../secret.txt", "/..%2fsecret.txt", "/sub/../../x"} {
		rec := get(t, h, p)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
			t.Fatalf("%s 状态码 = %d，期望 404/400", p, rec.Code)
		}
	}
}

// TestRealPublicPagesServe 用仓库里真实的 public/ 打一遍：
// 这是"页面全 404"事故的端到端回归——只要 baseDir 解析对，所有前台页面和资源都该 200。
func TestRealPublicPagesServe(t *testing.T) {
	root := repoRoot(t)
	pub := filepath.Join(root, "public")
	entries, err := os.ReadDir(pub)
	if err != nil {
		t.Skipf("没有 public/ 目录，跳过: %v", err)
	}

	old := baseDir
	baseDir = root
	t.Cleanup(func() { baseDir = old })

	if got := publicDir(); got != pub {
		t.Fatalf("publicDir() = %q，期望 %q", got, pub)
	}
	h := newStaticHandler(publicDir())

	// 首页单独断言（走目录分支）
	if rec := get(t, h, "/"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("/ = %d，首页未正常返回", rec.Code)
	}

	checked := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".html") && !strings.HasSuffix(name, ".css") &&
			!strings.HasSuffix(name, ".js") && !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".svg") {
			continue
		}
		if rec := get(t, h, "/"+name); rec.Code != http.StatusOK {
			// index.html 会被 http.ServeFile 规范化重定向到目录（301），这是正常行为
			if name == "index.html" && rec.Code == http.StatusMovedPermanently {
				checked++
				continue
			}
			t.Errorf("/%s 状态码 = %d，期望 200", name, rec.Code)
		}
		checked++
	}
	if checked < 40 {
		t.Fatalf("只检查了 %d 个前端文件，public/ 可能不完整", checked)
	}

	// 真缺页时依然是品牌 404（不是 200）
	if rec := get(t, h, "/definitely-not-here.html"); rec.Code != http.StatusNotFound {
		t.Fatalf("缺页状态码 = %d，期望 404", rec.Code)
	}
}
