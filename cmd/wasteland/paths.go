// paths.go — 站点根目录契约。
//
// 背景：入口从仓库根搬到 cmd/wasteland/ 之后，public/、tool-data/、dist/、_csv/
// 这些运行时资源仍然是相对当前工作目录的裸路径。一旦不是"从仓库根启动"，
// 静态页面就静默 404（工具页因为 go:embed 编进了二进制，仍然正常），
// 排障时极容易被误导成"路由写错了"。
//
// 现在统一由 baseDir 决定一切：启动时解析一次，失败直接报错退出，
// 不再让站点带着半个空壳跑起来。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// baseDir 是含 public/ 的站点目录，只在 main() 里解析一次。
// 为空时（单元测试等场景）各 helper 退回原来的相对路径，行为与改造前一致。
var baseDir string

// resolveBaseDir 依次尝试 APP_DIR、可执行文件所在目录、当前工作目录，
// 返回第一个能找到 public/index.html 的目录。
func resolveBaseDir() (string, error) {
	var tried []string
	probe := func(dir, source string) (string, bool) {
		if strings.TrimSpace(dir) == "" {
			return "", false
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", false
		}
		tried = append(tried, source+"="+abs)
		fi, err := os.Stat(filepath.Join(abs, "public", "index.html"))
		if err == nil && !fi.IsDir() {
			return abs, true
		}
		return "", false
	}

	// 1. 显式指定优先；指定了却不对，直接报错而不是悄悄换一个目录。
	if v := strings.TrimSpace(os.Getenv("APP_DIR")); v != "" {
		if dir, ok := probe(v, "APP_DIR"); ok {
			return dir, nil
		}
		return "", fmt.Errorf("APP_DIR=%s 下没有 public/index.html", v)
	}
	// 2. 可执行文件同目录：生产部署（程序与 public/ 并排）靠这条兜底。
	if exe, err := os.Executable(); err == nil {
		if dir, ok := probe(filepath.Dir(exe), "可执行文件目录"); ok {
			return dir, nil
		}
	}
	// 3. 当前工作目录：go run / 从仓库根启动时走这条。
	if wd, err := os.Getwd(); err == nil {
		if dir, ok := probe(wd, "当前目录"); ok {
			return dir, nil
		}
	}
	return "", fmt.Errorf("未找到 public/index.html（已尝试 %s）；请设置 APP_DIR 指向站点根目录",
		strings.Join(tried, "、"))
}

// inBase 把相对路径拼到站点根目录上。
func inBase(parts ...string) string {
	return filepath.Join(append([]string{baseDir}, parts...)...)
}

// publicDir 前端静态目录。
func publicDir() string { return inBase("public") }

// toolsRootDir 工具静态包目录（目前仅 nextjlc 预留，线上用 embed）。
func toolsRootDir() string { return inBase("dist", "tools") }

// dataDir 工具运行时数据（旧 JSON 快照，已迁 SQLite）。
func dataDir() string { return inBase("tool-data") }

// defaultCSVDir 课表 CSV 目录：默认在站点根目录的上一级（_csv 不进代码仓）。
func defaultCSVDir() string { return inBase("..", "_csv") }

// defaultAuthDBPath 用户数据库默认位置：站点根目录下，不要落到启动目录里。
func defaultAuthDBPath() string { return inBase("qdu-auth.db") }
