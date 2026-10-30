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
	Name string
	Data []byte
}

// collectUpload 解析一个上传项；zip 会被展开。
func collectUpload(name string, data []byte) ([]uploaded, error) {
	if len(data) >= 4 && bytes.HasPrefix(data, []byte("PK\x03\x04")) || strings.HasSuffix(strings.ToLower(name), ".zip") {
		return expandZip(data)
	}
	return []uploaded{{Name: name, Data: data}}, nil
}

// expandZip 展开压缩包（只读内容，不落盘，免疫 zip-slip）。
func expandZip(data []byte) ([]uploaded, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("压缩包无法读取")
	}
	if len(zr.File) > maxZipEntries {
		return nil, errors.New("压缩包里文件太多")
	}
	var out []uploaded
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		base := path.Base(zf.Name)
		if strings.HasPrefix(base, ".") {
			continue // 跳过 __MACOSX/._xxx 这类
		}
		rc, err := zf.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(io.LimitReader(rc, maxEntryBytes))
		_ = rc.Close()
		if err != nil || len(content) == 0 {
			continue
		}
		out = append(out, uploaded{Name: base, Data: content})
	}
	if len(out) == 0 {
		return nil, errors.New("压缩包里没有文件")
	}
	return out, nil
}

func headOf(data []byte) []byte {
	if len(data) > 4096 {
		return data[:4096]
	}
	return data
}
