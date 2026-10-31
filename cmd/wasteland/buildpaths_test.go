// buildpaths_test.go — 守住"构建入口"这条线。
//
// 2026-10 的教训：入口从仓库根搬到 cmd/wasteland/ 后，代码能编译，但
// start-server.ps1 / ci-local.ps1 / ci.yml / deploy/update.sh / 文档里的
// `go build ... .` 全部指向已经消失的根包。本地看起来"只是起不来"，
// 线上则是 update.sh 编译失败后自动回滚——症状和原因隔得很远。
//
// 这个测试把"构建入口必须指向 ./cmd/wasteland"钉死，顺带拦截 g→o 这类
// 批量替换事故（`oo build` / `oo vet` 这种调用根本不存在）。
package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRoot 从包目录往上找到含 go.mod 的仓库根。
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("向上找不到 go.mod，测试位置异常")
	return ""
}

// buildEntryfiles 是所有会调用 go build 的文件（脚本、CI、文档）。
var buildEntryfiles = []string{
	"start-server.ps1",
	"ci-local.ps1",
	"start-tools-local.ps1",
	"start-tools-admin-test.ps1",
	"deploy/update.sh",
	".github/workflows/ci.yml",
	"README.md",
	"DEPLOY-GUIDE.md",
}

// 以裸 "."（根包）结尾的 go build：2026-10 之后必然失败。
var rootPackageBuild = regexp.MustCompile(`go build\b[^\n]*[ \t]\.([ \t]*&)?[ \t]*\r?$`)

// g→o 事故签名：这些子命令不存在，出现就说明被批量替换过。
var gToOCorruption = regexp.MustCompile(`\boo (build|vet|test|version|mod|run|install)\b`)

func TestBuildEntrypointsUseCmdWasteland(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range buildEntryfiles {
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("读 %s: %v", rel, err)
		}
		text := string(raw)
		lines := strings.Split(text, "\n")

		hasCmd := false
		for i, line := range lines {
			line = strings.TrimRight(line, "\r")
			if rootPackageBuild.MatchString(line) {
				t.Errorf("%s:%d 构建入口仍指向根包（应写 ./cmd/wasteland）: %s", rel, i+1, strings.TrimSpace(line))
			}
			if gToOCorruption.MatchString(line) {
				t.Errorf("%s:%d 疑似 g→o 批量替换事故: %s", rel, i+1, strings.TrimSpace(line))
			}
			if strings.Contains(line, "go build") && strings.Contains(line, "./cmd/wasteland") {
				hasCmd = true
			}
		}
		if !hasCmd {
			t.Errorf("%s 里没有任何指向 ./cmd/wasteland 的 go build，入口可能又被改回根包了", rel)
		}
	}
}
