// virus_test.go — 上传安全防护单元测试（V12 / H4）
// 运行：go test -run TestVirus -v
// ClamAV 集成测试：本机设置 CLAMAV_CMD 后才会真正调用扫描（未设置自动跳过）。
package main

import (
	"os"
	"strings"
	"testing"
)

func TestDangerousExt(t *testing.T) {
	bad := []string{"evil.exe", "run.bat", "x.ps1", "doc.docm", "page.svg", "s.sh"}
	for _, name := range bad {
		if ok, _ := checkDangerousFile(name, []byte("hello")); !ok {
			t.Errorf("应拦截危险扩展名: %s", name)
		}
	}
	good := []string{"note.md", "book.pdf", "photo.jpg", "table.xlsx", "code.go", "essay.docx"}
	for _, name := range good {
		if ok, _ := checkDangerousFile(name, []byte("hello")); ok {
			t.Errorf("不应拦截正常扩展名: %s", name)
		}
	}
}

func TestDangerousMagic(t *testing.T) {
	// 伪装成 jpg 的 PE 可执行文件（MZ 头）
	pe := append([]byte("MZ"), []byte{0x90, 0x00, 0x03, 0x00}...)
	if ok, _ := checkDangerousFile("innocent.jpg", pe); !ok {
		t.Error("应拦截 MZ 头伪装文件")
	}
	// ELF
	if ok, _ := checkDangerousFile("data.bin", []byte{0x7F, 'E', 'L', 'F', 2, 1, 1}); !ok {
		t.Error("应拦截 ELF 头")
	}
	// 普通文本不以 #! 开头则放行
	if ok, _ := checkDangerousFile("note.md", []byte("# 标题\n正文")); ok {
		t.Error("正常 md 不应被拦")
	}
	// #! 开头的 .py 应拦截
	if ok, _ := checkDangerousFile("evil.py", []byte("#!/usr/bin/env python\nprint(1)")); !ok {
		t.Error("shebang 脚本应被拦")
	}
}

// TestClamAVIntegration 集成测试：设置 CLAMAV_CMD 后对 EICAR 测试串应判为病毒；
// 未设置时跳过（不阻塞 CI / 开发机）。
func TestClamAVIntegration(t *testing.T) {
	cmd := os.Getenv("CLAMAV_CMD")
	if cmd == "" {
		t.Skip("未设置 CLAMAV_CMD，跳过 ClamAV 集成测试（生产部署后设置即可启用）")
	}
	eicar := []byte("X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*")
	clean, msg := clamavScan(eicar, "eicar-test.txt")
	if clean {
		t.Fatal("EICAR 测试文件应被判为病毒，但扫描放行了")
	}
	if !strings.Contains(msg, "EICAR") {
		t.Logf("命中信息: %s", msg)
	}
}
