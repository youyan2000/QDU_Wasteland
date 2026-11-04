package nextjlc

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// buildZip 构造一个内存 zip：每项 1MB 的全零内容，压缩后极小（典型 zip 炸弹形态）。
func buildZip(t *testing.T, entries int, size int) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	block := make([]byte, size)
	for i := 0; i < entries; i++ {
		w, err := zw.Create("board/gerber_" + string(rune('a'+i%26)) + ".gbr")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExpandZipRejectsCompressionBomb(t *testing.T) {
	data := buildZip(t, 4, 1<<20)
	if len(data) > 1<<20 {
		t.Fatalf("构造的测试包应该很小，实际 %d", len(data))
	}
	_, err := expandZip(data)
	if err == nil {
		t.Fatal("高压缩比压缩包应被拒绝")
	}
	if !strings.Contains(err.Error(), "压缩比") {
		t.Fatalf("错误信息应提到压缩比，实际: %v", err)
	}
}

func TestExpandZipRejectsTooManyEntries(t *testing.T) {
	data := buildZip(t, maxZipEntries+1, 16)
	if _, err := expandZip(data); err == nil {
		t.Fatal("条目过多应被拒绝")
	}
}

func TestExpandZipAcceptsNormalPackage(t *testing.T) {
	// 正常 Gerber 包：几个小文件，压缩比也正常。
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"board.GTL", "board.GBL", "board.GM1"} {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte("%FSLAX46Y46*%\n%MOMM*%\n" + strings.Repeat("X100Y200D01*\n", 50)))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := expandZip(buf.Bytes())
	if err != nil {
		t.Fatalf("正常包不应被拒绝: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("应解出 3 个文件，实际 %d", len(files))
	}
}
