package history

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func readZipEntries(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip 读取失败: %v", err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(body)
	}
	return out
}

func TestExtensionZipChoosesManifestPerBrowser(t *testing.T) {
	chrome := readZipEntries(t, mustZip(t, "chrome"))
	ff := readZipEntries(t, mustZip(t, "firefox"))

	if _, ok := chrome["manifest.chrome.json"]; ok {
		t.Fatal("压缩包里不应带原始清单文件")
	}
	if !strings.Contains(chrome["manifest.json"], `"service_worker"`) {
		t.Fatalf("Chrome 清单应使用 service_worker:\n%s", chrome["manifest.json"])
	}
	if !strings.Contains(ff["manifest.json"], `"scripts"`) ||
		!strings.Contains(ff["manifest.json"], "browser_specific_settings") {
		t.Fatalf("Firefox 清单应使用 background.scripts + gecko 设置:\n%s", ff["manifest.json"])
	}
	for name := range chrome {
		if name == "manifest.json" {
			continue
		}
		if _, ok := ff[name]; !ok {
			t.Fatalf("两个包都应包含 %s", name)
		}
	}
}

func mustZip(t *testing.T, browser string) []byte {
	t.Helper()
	data, err := ExtensionZip(browser)
	if err != nil || len(data) < 500 {
		t.Fatalf("ExtensionZip(%q): %v len=%d", browser, err, len(data))
	}
	return data
}
