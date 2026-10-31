// paths_test.go — 覆盖"站点根目录从哪来"这一契约。
//
// 这是本次事故的根因：入口搬进 cmd/wasteland/ 后，资源路径还靠工作目录。
// 这些用例保证以后无论怎么挪入口，根目录解析都有据可依、失败会报错。
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// mkSite 造一个最小的站点目录（含 public/index.html）。
func mkSite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "public"), 0o755); err != nil {
		t.Fatalf("建 public 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "public", "index.html"), []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatalf("写 index.html 失败: %v", err)
	}
	return dir
}

func mustDir(t *testing.T, got, want string) {
	t.Helper()
	g, err := filepath.Abs(got)
	if err != nil {
		t.Fatalf("Abs(%q): %v", got, err)
	}
	w, err := filepath.Abs(want)
	if err != nil {
		t.Fatalf("Abs(%q): %v", want, err)
	}
	if g != w {
		t.Fatalf("根目录 = %q，期望 %q", g, w)
	}
}

// APP_DIR 指定正确时应直接采用。
func TestResolveBaseDirFromEnv(t *testing.T) {
	dir := mkSite(t)
	t.Setenv("APP_DIR", dir)
	got, err := resolveBaseDir()
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	mustDir(t, got, dir)
}

// APP_DIR 指错时必须报错，不能悄悄退回别的目录（否则又变成静默 404）。
func TestResolveBaseDirEnvInvalid(t *testing.T) {
	t.Setenv("APP_DIR", t.TempDir()) // 空目录，没有 public/index.html
	if _, err := resolveBaseDir(); err == nil {
		t.Fatal("APP_DIR 无效时应返回错误")
	}
}

// 没设 APP_DIR 时回退到当前工作目录（go run / 从仓库根启动）。
func TestResolveBaseDirFromCwd(t *testing.T) {
	dir := mkSite(t)
	t.Setenv("APP_DIR", "")
	t.Chdir(dir)
	got, err := resolveBaseDir()
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	mustDir(t, got, dir)
}

// 完全找不到时错误信息里要带上尝试过的目录，便于排障。
func TestResolveBaseDirNothingFound(t *testing.T) {
	t.Setenv("APP_DIR", "")
	t.Chdir(t.TempDir())
	_, err := resolveBaseDir()
	if err == nil {
		t.Fatal("空目录下应返回错误")
	}
	if got := err.Error(); !contains(got, "public/index.html") || !contains(got, "APP_DIR") {
		t.Fatalf("错误信息缺少可操作提示: %v", got)
	}
}

// 各 helper 都必须跟着 baseDir 走。
func TestPathHelpersFollowBaseDir(t *testing.T) {
	old := baseDir
	t.Cleanup(func() { baseDir = old })

	root := t.TempDir()
	baseDir = root

	cases := map[string]string{
		publicDir():           filepath.Join(root, "public"),
		toolsRootDir():        filepath.Join(root, "dist", "tools"),
		dataDir():             filepath.Join(root, "tool-data"),
		defaultAuthDBPath():   filepath.Join(root, "qdu-auth.db"),
		defaultCSVDir():       filepath.Join(root, "..", "_csv"),
	}
	for got, want := range cases {
		if filepath.Clean(got) != filepath.Clean(want) {
			t.Fatalf("%q 期望 %q", got, want)
		}
	}
}

// baseDir 为空（单元测试场景）时应退回改造前的相对路径。
func TestPathHelpersWithoutBaseDir(t *testing.T) {
	old := baseDir
	t.Cleanup(func() { baseDir = old })
	baseDir = ""

	if got := publicDir(); got != "public" {
		t.Fatalf("publicDir() = %q", got)
	}
	if got := defaultCSVDir(); got != filepath.Join("..", "_csv") {
		t.Fatalf("defaultCSVDir() = %q", got)
	}
	if got := defaultAuthDBPath(); got != "qdu-auth.db" {
		t.Fatalf("defaultAuthDBPath() = %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
