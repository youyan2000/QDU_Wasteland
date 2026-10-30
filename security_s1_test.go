// security_s1_test.go — S1 上线阻断修复的回归测试。
//
// 覆盖：
//   P0-1 验证码：SVG 里不得出现可读文本节点；校验失败即作废
//   P0-2 限流：clientIP 不再信任入站 X-Forwarded-For
//   P1-1 相册：targetId / targetType 白名单校验
//   P1-2 SSRF：内网/元数据地址一律拒绝，站内路径只读白名单目录
package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------- P0-1 验证码 ----------

func TestCaptchaSVGHasNoReadableTextNode(t *testing.T) {
	code := "AB23"
	svg := svgCaptcha(code)

	if strings.Contains(svg, "<text") {
		t.Fatal("SVG 中不允许出现 <text> 节点（答案会被一次 GET 抠走）")
	}
	// 逐个字符都不应以裸文本形式出现（形如 >A<）。
	for _, ch := range code {
		if strings.Contains(svg, ">"+string(ch)+"<") {
			t.Fatalf("答案字符 %q 以文本形式出现在 SVG 中", ch)
		}
	}
	if !strings.Contains(svg, "<polyline") {
		t.Fatal("字符应当用 <polyline> 笔画绘制")
	}
}

func TestCaptchaInvalidatedOnWrongAnswer(t *testing.T) {
	id, code := capStore()

	if verifyCaptchaCode(id, "____") {
		t.Fatal("错误答案不应通过")
	}
	// 一次失败之后，即使答案正确也必须失效，杜绝同一张图反复猜。
	if verifyCaptchaCode(id, code) {
		t.Fatal("失败的验证码必须立即作废，不能再用正确答案通过")
	}
}

func TestCaptchaConsumedOnSuccess(t *testing.T) {
	id, code := capStore()
	if !verifyCaptchaCode(id, code) {
		t.Fatal("正确答案应通过")
	}
	if verifyCaptchaCode(id, code) {
		t.Fatal("一次性：通过后同一 id 不得再用")
	}
}

// ---------- P0-2 客户端 IP ----------

func TestClientIPIgnoresForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	r.RemoteAddr = "203.0.113.9:51234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("X-Real-IP", "5.6.7.8")

	if got := clientIP(r); got != "203.0.113.9" {
		t.Fatalf("非本机对端不得采信转发头，got %q", got)
	}
}

func TestClientIPUsesRealIPBehindLocalProxy(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	r.RemoteAddr = "127.0.0.1:51234"
	r.Header.Set("X-Real-IP", "198.51.100.7")
	r.Header.Set("X-Forwarded-For", "1.2.3.4")

	if got := clientIP(r); got != "198.51.100.7" {
		t.Fatalf("本机反代应采信 X-Real-IP，got %q", got)
	}
}

func TestClientIPRejectsGarbageRealIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	r.RemoteAddr = "127.0.0.1:51234"
	r.Header.Set("X-Real-IP", "not-an-ip")

	if got := clientIP(r); got != "127.0.0.1" {
		t.Fatalf("非法 X-Real-IP 应回退到 RemoteAddr，got %q", got)
	}
}

// ---------- P1-1 相册入册校验 ----------

func TestAlbumTargetValidation(t *testing.T) {
	cases := []struct {
		typ, id string
		want    bool
	}{
		{"post", "123", true},
		{"article", "1", true},
		{"file", "99", true},
		{"course", "4081120203018", true},
		{"photo", "al_1700000000_1.jpg", true},
		{"post", `1" onerror="alert(1)`, false},
		{"post", "1;drop", false},
		{"photo", `x" onerror="alert(1)`, false},
		{"photo", "../etc/passwd", false},
		{"unknown", "1", false},
		{"post", "", false},
	}
	for _, c := range cases {
		ok := albumTargetTypes[c.typ]
		if ok {
			switch c.typ {
			case "photo":
				ok = isPhotoName(c.id)
			case "course":
				ok = c.id != "" && len(c.id) <= 40
			default:
				ok = isDigits(c.id)
			}
		}
		if ok != c.want {
			t.Errorf("type=%q id=%q 期望 %v，实际 %v", c.typ, c.id, c.want, ok)
		}
	}
}

// ---------- P1-2 SSRF ----------

func TestIsPublicIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1",
		"169.254.169.254", "100.64.0.1", "0.0.0.0", "::1", "fc00::1", "fe80::1",
	}
	for _, s := range blocked {
		if isPublicIP(net.ParseIP(s)) {
			t.Errorf("%s 应被拒绝", s)
		}
	}
	allowed := []string{"8.8.8.8", "1.1.1.1", "203.0.113.9"}
	for _, s := range allowed {
		if !isPublicIP(net.ParseIP(s)) {
			t.Errorf("%s 应被允许", s)
		}
	}
}

func TestReadInternalImageRejectsTraversal(t *testing.T) {
	for _, p := range []string{
		"/album-photo-file/../secret.png",
		"/album-photo-file//etc/passwd",
		"/forum-image-file/..%2fsecret.png",
		"/etc/passwd",
		"/avatar-file/nested/x.png",
	} {
		if _, err := readInternalImage(p); err == nil {
			t.Errorf("%q 应被拒绝", p)
		}
	}
}

func TestParseHTTPURL(t *testing.T) {
	bad := []string{
		"file:///etc/passwd",
		"javascript:alert(1)",
		"gopher://127.0.0.1/",
		"http://localhost/x.png",
		"http://foo.internal/x.png",
		"",
	}
	for _, s := range bad {
		if _, err := parseHTTPURL(s); err == nil {
			t.Errorf("%q 应被拒绝", s)
		}
	}
	if _, err := parseHTTPURL("https://example.com/a.jpg"); err != nil {
		t.Errorf("正常 https 地址不应被拒: %v", err)
	}
}

func TestExtForImage(t *testing.T) {
	cases := map[string]string{
		"\xff\xd8\xff\xe0rest":               ".jpg",
		"\x89PNG\r\n\x1a\nrest":              ".png",
		"GIF89a":                             ".gif",
		"RIFF\x00\x00\x00\x00WEBPrest":       ".webp",
		"<html>not an image</html>":          "",
	}
	for in, want := range cases {
		if got := extForImage([]byte(in)); got != want {
			t.Errorf("数据 %q 期望 %q，实际 %q", in, want, got)
		}
	}
}
