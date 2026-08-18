// captcha.go — 图形验证码（SVG 实现，无字体依赖）
// GET /api/captcha/  生成一张 SVG 验证码，答案存内存(30分钟/一次性)，返回 {id}
// GET /api/captcha/{id}  返回该验证码的 SVG 图（<img src> 用）
// POST /api/captcha/verify  {id, answer}  校验
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	crand "crypto/rand"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type captchaEntry struct {
	answer string
	expire int64 // 毫秒
}

var (
	capMu    sync.Mutex
	captchas = map[string]*captchaEntry{}
)

func genCapCode() string {
	const set = "ABCDEFGHJKMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789" // 去掉易混淆 I/L/O/0/1
	b := make([]byte, 4)
	crand.Read(b)
	var s []byte
	for _, v := range b {
		s = append(s, set[int(v)%len(set)])
	}
	return string(s)
}

func capStore() (id, code string) {
	idB := make([]byte, 12)
	crand.Read(idB)
	id = hex.EncodeToString(idB)
	code = genCapCode()
	capMu.Lock()
	captchas[id] = &captchaEntry{answer: code, expire: time.Now().Add(30 * time.Minute).UnixMilli()}
	capMu.Unlock()
	// 惰性清理
	if len(captchas) > 5000 {
		now := time.Now().UnixMilli()
		capMu.Lock()
		for k, e := range captchas {
			if e.expire < now {
				delete(captchas, k)
			}
		}
		capMu.Unlock()
	}
	return id, code
}

func capGet(id string) string {
	capMu.Lock()
	defer capMu.Unlock()
	e, ok := captchas[id]
	if !ok || e.expire < time.Now().UnixMilli() {
		return ""
	}
	return e.answer
}

// randN 安全伪随机 [0,n)
func randN(n int) int { return rand.Intn(n) }

// svgCaptcha 生成 SVG 字符串
func svgCaptcha(code string) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	w, h := 150, 50
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, w, h, w, h))
	sb.WriteString(`<rect width="100%" height="100%" fill="#faf6f2"/>`)
	// 干扰线
	for i := 0; i < 4; i++ {
		x1, y1 := r.Intn(w), r.Intn(h)
		x2, y2 := r.Intn(w), r.Intn(h)
		sb.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#e0d6cc" stroke-width="1"/>`, x1, y1, x2, y2))
	}
	// 噪点
	for i := 0; i < 40; i++ {
		sb.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="1" fill="#c9beb2"/>`, r.Intn(w), r.Intn(h)))
	}
	// 字符（每个随机旋转/偏移/颜色）
	start := 20
	for i, ch := range code {
		ang := r.Intn(40) - 20
		x := start + i*32
		y := 32 + r.Intn(8)
		color := fmt.Sprintf("#%02x%02x%02x", 26+r.Intn(40), 60+r.Intn(60), 90+r.Intn(60))
		sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" font-size="30" font-weight="700" font-family="Arial" fill="%s" transform="rotate(%d %d %d)">%c</text>`, x, y, color, ang, x, y, ch))
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

// handleNewCaptcha 生成并返回 {id}
func handleNewCaptcha() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := capStore()
		apiJSON(w, 200, map[string]any{"id": id})
	}
}

// handleCaptchaImage 返回 SVG 图（需要 ?id= 或 /{id}）
func handleCaptchaImage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/api/captcha/")
		id := strings.TrimSuffix(p, ".svg")
		code := capGet(id)
		if code == "" {
			apiErr(w, 404, "验证码已失效")
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(svgCaptcha(code)))
	}
}

// verifyCaptchaCode 校验验证码并作废（供登录/注册/发帖等复用）
func verifyCaptchaCode(id, answer string) bool {
	if id == "" || answer == "" {
		return false
	}
	code := capGet(id)
	if code == "" {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(answer), code) {
		return false
	}
	capMu.Lock()
	delete(captchas, id)
	capMu.Unlock()
	return true
}

// handleVerifyCaptcha 校验验证码
func handleVerifyCaptcha() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID     string `json:"id"`
			Answer string `json:"answer"`
		}
		if err := jsonUnmarshal(r, &body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		code := capGet(body.ID)
		if code == "" {
			apiErr(w, 410, "验证码已失效，请刷新")
			return
		}
		if !strings.EqualFold(strings.TrimSpace(body.Answer), code) {
			apiErr(w, 400, "验证码错误")
			return
		}
		capMu.Lock()
		delete(captchas, body.ID)
		capMu.Unlock()
		apiJSON(w, 200, map[string]any{"ok": true})
	}
}

func jsonUnmarshal(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}