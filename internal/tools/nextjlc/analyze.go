// analyze.go — 上传解析：把文件（或 zip 里的文件）收集成「名字 + 内容」。
package nextjlc

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"path"
	"strings"
)

const (
	maxUploadBytes = 64 << 20
	maxZipEntries  = 400
	maxEntryBytes  = 32 << 20
)

// uploaded 是一个待处理的文件。
type uploaded struct {
	Name  string
	Data  []byte
	Drill bool // 按内容识别出的钻孔文件（Altium 常叫 board.TXT）
}

// collectUpload 解析一个上传项；zip 会被展开。
func collectUpload(name string, data []byte) ([]uploaded, error) {
	if len(data) >= 4 && bytes.HasPrefix(data, []byte("PK\x03\x04")) || strings.HasSuffix(strings.ToLower(name), ".zip") {
		return expandZip(data)
	}
	return []uploaded{{Name: name, Data: data}}, nil
}

// expandZip 展开压缩包（只读内容，不落盘，免疫 zip-slip）。
// 结构规则对齐原版 web/upload.tsx：
//  1. 根目录直接有 ≥2 个文件 → 只取根目录文件；
//  2. 根目录没有文件、且只有一个顶层文件夹 → 取该文件夹下（不再下探）的文件；
//  3. 其它情况（多个顶层目录 / 深层嵌套 / 文件不足 2 个）→ 报错，避免多块板混包。
func expandZip(data []byte) ([]uploaded, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("压缩包无法读取")
	}
	if len(zr.File) > maxZipEntries {
		return nil, errors.New("压缩包里文件太多")
	}

	type entry struct {
		name string
		file *zip.File
	}
	var entries []entry
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		name := strings.ReplaceAll(zf.Name, "\\", "/")
		base := path.Base(name)
		if strings.HasPrefix(base, ".") {
			continue // __MACOSX/._xxx
		}
		entries = append(entries, entry{name: name, file: zf})
	}

	// 1) 根目录文件
	var picked []entry
	for _, e := range entries {
		if !strings.Contains(e.name, "/") {
			picked = append(picked, e)
		}
	}

	// 2) 只有一个顶层文件夹时，取它直接子文件
	if len(picked) < 2 {
		top := map[string]bool{}
		for _, e := range entries {
			if i := strings.Index(e.name, "/"); i > 0 {
				top[e.name[:i]] = true
			}
		}
		if len(top) == 1 {
			var folder string
			for k := range top {
				folder = k + "/"
			}
			picked = picked[:0]
			for _, e := range entries {
				if strings.HasPrefix(e.name, folder) {
					rest := strings.TrimPrefix(e.name, folder)
					if !strings.Contains(rest, "/") {
						picked = append(picked, e)
					}
				}
			}
		}
	}

	if len(picked) < 2 {
		return nil, errors.New("压缩包结构不正确：请确保根目录（或唯一的那个文件夹）里至少有两个 Gerber 文件")
	}

	out := make([]uploaded, 0, len(picked))
	for _, e := range picked {
		rc, err := e.file.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(io.LimitReader(rc, maxEntryBytes))
		_ = rc.Close()
		if err != nil || len(content) == 0 {
			continue
		}
		out = append(out, uploaded{Name: path.Base(e.name), Data: content})
	}
	if len(out) < 2 {
		return nil, errors.New("压缩包里可用的文件不足两个")
	}
	return out, nil
}

func headOf(data []byte) []byte {
	if len(data) > 4096 {
		return data[:4096]
	}
	return data
}
