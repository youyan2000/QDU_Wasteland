// Command repocheck 检查仓库里是否混入了不该跟踪的东西：
// 截图/大文件/二进制、凭据文件名、运行时目录。
//
// 用法：
//
//	go run ./cmd/repocheck            # 检查 HEAD 里的已跟踪文件（CI 用）
//	go run ./cmd/repocheck --staged   # 只检查本次暂存的文件（pre-commit 用）
//
// 退出码 1 表示发现问题。CI、local CI 与 git pre-commit 都调用它，
// 这样即使有人用 --no-verify 绕过钩子，云端仍会拦下来。
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

const maxFileSize = 2 << 20 // 2 MiB

// 允许出现在仓库里的图片/二进制目录（站点资源与工具封面）。
var allowedBinaryPrefixes = []string{
	"public/",
	"internal/tools/static/covers/",
}

// 一律不允许跟踪的目录。
var blockedPrefixes = []string{
	".taste-skill/",
	".cache/",
	"backups/",
	"dist/",
	"node_modules/",
	"VPN/",
	"tool-data/",
	"uploads/",
}

var blockedExt = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	".bmp": true, ".tiff": true, ".psd": true,
	".mp4": true, ".mov": true, ".mp3": true, ".wav": true,
	".zip": true, ".7z": true, ".rar": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true,
	".db": true, ".sqlite": true,
}

type fileEntry struct {
	Path string
	Size int64
}

func main() {
	staged := flag.Bool("staged", false, "只检查 git 暂存区里的文件")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "repocheck: 不在 git 仓库里:", err)
		os.Exit(2)
	}
	if err := os.Chdir(root); err != nil {
		fmt.Fprintln(os.Stderr, "repocheck:", err)
		os.Exit(2)
	}

	var files []fileEntry
	if *staged {
		files, err = stagedFiles()
	} else {
		files, err = headFiles()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "repocheck: 读取文件列表失败:", err)
		os.Exit(2)
	}

	problems := check(files)
	if len(problems) == 0 {
		fmt.Printf("仓库卫生检查通过（检查 %d 个文件）\n", len(files))
		return
	}
	fmt.Fprintln(os.Stderr, "仓库卫生检查未通过：")
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, "  -", p)
	}
	fmt.Fprintln(os.Stderr, "\n处理建议：")
	fmt.Fprintln(os.Stderr, "  git rm -r --cached <路径>   # 停止跟踪（本地文件保留）")
	fmt.Fprintln(os.Stderr, "  然后把目录/后缀加进 .gitignore，再提交")
	os.Exit(1)
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func headFiles() ([]fileEntry, error) {
	out, err := exec.Command("git", "ls-tree", "-r", "-l", "-z", "HEAD").Output()
	if err != nil {
		return nil, err
	}
	var files []fileEntry
	for _, rec := range bytes.Split(out, []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		// 格式：<mode> SP <type> SP <sha> SP* <size> TAB <path>
		tab := bytes.IndexByte(rec, '\t')
		if tab < 0 {
			continue
		}
		meta := strings.Fields(string(rec[:tab]))
		if len(meta) < 4 || meta[1] != "blob" {
			continue
		}
		size, _ := strconv.ParseInt(meta[3], 10, 64)
		files = append(files, fileEntry{Path: string(rec[tab+1:]), Size: size})
	}
	return files, nil
}

func stagedFiles() ([]fileEntry, error) {
	out, err := exec.Command("git", "diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z").Output()
	if err != nil {
		return nil, err
	}
	var files []fileEntry
	for _, p := range bytes.Split(out, []byte{0}) {
		if len(p) == 0 {
			continue
		}
		name := string(p)
		size := int64(0)
		if info, err := os.Stat(name); err == nil {
			size = info.Size()
		}
		files = append(files, fileEntry{Path: name, Size: size})
	}
	return files, nil
}

func check(files []fileEntry) []string {
	var problems []string
	for _, f := range files {
		p := strings.TrimPrefix(path.Clean(f.Path), "./")
		lower := strings.ToLower(p)

		for _, prefix := range blockedPrefixes {
			if strings.HasPrefix(p, prefix) {
				problems = append(problems, fmt.Sprintf("%s：位于不应入库的目录 %s", p, prefix))
			}
		}

		if f.Size > maxFileSize {
			problems = append(problems, fmt.Sprintf("%s：文件 %.1fMB，超过 %.0fMB 上限（截图/产物请放仓库外）",
				p, float64(f.Size)/(1<<20), float64(maxFileSize)/(1<<20)))
		}

		ext := strings.ToLower(path.Ext(lower))
		if blockedExt[ext] && !underAny(p, allowedBinaryPrefixes) {
			problems = append(problems, fmt.Sprintf("%s：%s 只允许放在 public/ 或 internal/tools/static/covers/ 下", p, ext))
		}

		if looksLikeSecretName(path.Base(lower)) {
			problems = append(problems, fmt.Sprintf("%s：文件名像凭据，不允许入库", p))
		}
	}
	return problems
}

func underAny(p string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func looksLikeSecretName(base string) bool {
	if base == "" {
		return false
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".env") {
		return true
	}
	if strings.HasPrefix(base, "id_rsa") || strings.Contains(base, "credentials") {
		return true
	}
	switch path.Ext(base) {
	case ".key", ".pem", ".p12", ".pfx":
		return true
	}
	return false
}
