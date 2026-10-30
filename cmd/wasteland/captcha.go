// captcha.go — 图形验证码（SVG 实现，无字体依赖）
//
// GET  /api/captcha/          生成一张验证码，返回 {id}
// GET  /api/captcha/{id}.svg  返回该验证码的图（<img src> 用）
// POST /api/captcha/verify    {id, answer} 校验
//
// 安全要点（见 docs/SECURITY-AUDIT-2026-10-31.md P0-1）：
//  1. 字符用「线段笔画」绘制，SVG 里不存在任何可读文本节点 —— 早期版本把每个字符
//     写成 <text>%c</text>，一次 GET 就能把答案正则抠出来，验证码形同虚设。
//  2. 校验失败即作废该 id，避免同一张图被反复猜。
//  3. 创建接口按指纹限流，过期条目由后台定时器清理（不再惰性堆积）。
//  4. 不再提供 DISABLE_CAPTCHA 之类的运行期关闭开关。
package main

import (
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// captchaTTL 验证码有效期。
const captchaTTL = 10 * time.Minute

// captchaCreateLimit 每指纹每小时可创建的验证码张数。
const captchaCreateLimit = 120

type captchaEntry struct {
	answer string
	expire int64 // 毫秒
}

var (
	capMu    sync.Mutex
	captchas = map[string]*captchaEntry{}
)

func init() {
	// 后台清理：不再依赖"创建到 5000 条才顺手扫一遍"。
	go func() {
		ticker := time.NewTicker(3 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now().UnixMilli()
			capMu.Lock()
			for k, e := range captchas {
				if e.expire < now {
					delete(captchas, k)
				}
			}
			capMu.Unlock()
		}
	}()
}

// genCapCode 生成 4 位答案。字符集去掉易混淆的 I/L/O/0/1。
func genCapCode() string {
	const set = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	b := make([]byte, 4)
	if _, err := crand.Read(b); err != nil {
		// 熵不可用时退化为时间种子，宁可弱一点也不要 panic。
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		for i := range b {
			b[i] = byte(r.Intn(256))
		}
	}
	var s []byte
	for _, v := range b {
		s = append(s, set[int(v)%len(set)])
	}
	return string(s)
}

func capStore() (id, code string) {
	idB := make([]byte, 12)
	if _, err := crand.Read(idB); err != nil {
		idB = []byte(strconv.FormatInt(time.Now().UnixNano(), 10))
	}
	id = hex.EncodeToString(idB)
	code = genCapCode()
	capMu.Lock()
	captchas[id] = &captchaEntry{answer: code, expire: time.Now().Add(captchaTTL).UnixMilli()}
	capMu.Unlock()
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

// capDrop 作废一张验证码（成功或被判错都调用）。
func capDrop(id string) {
	capMu.Lock()
	delete(captchas, id)
	capMu.Unlock()
}

// strokeGlyphs 是 3×5 网格上的笔画字形：
// 每个字形是若干条折线，折线由「x,y」点序列组成（x∈0..2, y∈0..4）。
// 用线段而不是文字节点渲染，SVG 源码里因此不含任何答案字符。
var strokeGlyphs = map[rune][]string{
	'A': {"0,4 0,1 1,0 2,1 2,4", "0,2 2,2"},
	'B': {"0,0 0,4", "0,0 2,0 2,2 0,2", "0,2 2,2 2,4 0,4"},
	'C': {"2,0 0,0 0,4 2,4"},
	'D': {"0,0 0,4", "0,0 2,1 2,3 0,4"},
	'E': {"2,0 0,0 0,4 2,4", "0,2 1,2"},
	'F': {"2,0 0,0 0,4", "0,2 1,2"},
	'G': {"2,0 0,0 0,4 2,4 2,2 1,2"},
	'H': {"0,0 0,4", "2,0 2,4", "0,2 2,2"},
	'J': {"2,0 2,4 0,4 0,3"},
	'K': {"0,0 0,4", "0,2 2,0", "0,2 2,4"},
	'M': {"0,4 0,0 1,2 2,0 2,4"},
	'N': {"0,4 0,0", "0,0 2,4", "2,4 2,0"},
	'P': {"0,4 0,0 2,0 2,2 0,2"},
	'Q': {"0,0 0,4 2,4 2,0 0,0", "1,2 2,4"},
	'R': {"0,4 0,0 2,0 2,2 0,2", "0,2 2,4"},
	'S': {"2,0 0,0 0,2 2,2 2,4 0,4"},
	'T': {"0,0 2,0", "1,0 1,4"},
	'U': {"0,0 0,4 2,4 2,0"},
	'V': {"0,0 1,4 2,0"},
	'W': {"0,0 0,4 1,2 2,4 2,0"},
	'X': {"0,0 2,4", "2,0 0,4"},
	'Y': {"0,0 1,2 2,0", "1,2 1,4"},
	'Z': {"0,0 2,0 0,4 2,4"},
	'2': {"0,0 2,0 2,2 0,2 0,4 2,4"},
	'3': {"0,0 2,0 2,4 0,4", "1,2 2,2"},
	'4': {"0,0 0,2 2,2", "2,0 2,4"},
	'5': {"2,0 0,0 0,2 2,2 2,4 0,4"},
	'6': {"2,0 0,0 0,4 2,4 2,2 0,2"},
	'7': {"0,0 2,0 1,4"},
	'8': {"0,0 2,0 2,4 0,4 0,0", "0,2 2,2"},
	'9': {"2,2 0,2 0,0 2,0 2,4"},
}

// svgCaptcha 生成 SVG 字符串。所有字符都以 <polyline> 笔画输出。
func svgCaptcha(code string) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	w, h := 168, 56
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="captcha">`, w, h, w, h))
	sb.WriteString(fmt.Sprintf(`<rect width="100%%" height="100%%" fill="#faf6f2"/>`))

	const cellW = 36  // 每个字符占宽
	const gridW = 22.0 // 字形宽度（像素）
	const gridH = 30.0 // 字形高度（像素）
	const padX = 14

	startX := float64(padX)
	baseY := (float64(h) - gridH) / 2

	for i, ch := range code {
		polys, ok := strokeGlyphs[ch]
		if !ok {
			continue
		}
		ang := r.Intn(30) - 15
		cellX := startX + float64(i)*cellW
		dy := float64(r.Intn(6) - 3)
		color := fmt.Sprintf("#%02x%02x%02x", 22+r.Intn(40), 52+r.Intn(60), 84+r.Intn(70))

		for _, poly := range polys {
			var pts []string
			for _, p := range strings.Split(poly, " ") {
				xy := strings.SplitN(p, ",", 2)
				if len(xy) != 2 {
					continue
				}
				gx, _ := strconv.ParseFloat(xy[0], 64)
				gy, _ := strconv.ParseFloat(xy[1], 64)
				// 网格 → 像素，并加入轻微抖动
				px := cellX + gx/2*gridW + float64(r.Intn(3)-1)
				py := baseY + gy/4*gridH + dy + float64(r.Intn(3)-1)
				pts = append(pts, fmt.Sprintf("%.1f,%.1f", px, py))
			}
			if len(pts) < 2 {
				continue
			}
			cx := cellX + gridW/2
			cy := baseY + gridH/2
			sb.WriteString(fmt.Sprintf(
				`<polyline points="%s" fill="none" stroke="%s" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" transform="rotate(%d %.1f %.1f)"/>`,
				strings.Join(pts, " "), color, ang, cx, cy))
		}
	}

	// 干扰线与噪点画在字形之后，压住笔画。
	for i := 0; i < 5; i++ {
		sb.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#ded2c6" stroke-width="1"/>`,
			r.Intn(w), r.Intn(h), r.Intn(w), r.Intn(h)))
	}
	for i := 0; i < 50; i++ {
		sb.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="1" fill="#c9beb2"/>`, r.Intn(w), r.Intn(h)))
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

// handleNewCaptcha 生成并返回 {id}（按指纹限流）。
func handleNewCaptcha() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ok, _ := rateLimitOK(fpKey(r)+"|captcha", captchaCreateLimit); !ok {
			w.Header().Set("Retry-After", "600")
			apiErr(w, 429, "获取验证码过于频繁，请稍后再试")
			return
		}
		id, _ := capStore()
		apiJSON(w, 200, map[string]any{"id": id})
	}
}

// handleCaptchaImage 返回 SVG 图（需要 /{id}.svg）
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
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// 每次取图重新随机化笔画抖动，避免同一 id 反复取图做差分识别。
		_, _ = w.Write([]byte(svgCaptcha(code)))
	}
}

// verifyCaptchaCode 校验验证码；无论成功还是失败，该 id 都立即作废。
func verifyCaptchaCode(id, answer string) bool {
	if id == "" || answer == "" {
		return false
	}
	code := capGet(id)
	if code == "" {
		return false
	}
	// 先作废再比对：并发猜同一个 id 时只有一个请求能拿到答案。
	capDrop(id)
	return strings.EqualFold(strings.TrimSpace(answer), code)
}

// handleVerifyCaptcha 校验验证码（同样一次性）
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
		capDrop(body.ID)
		if !strings.EqualFold(strings.TrimSpace(body.Answer), code) {
			apiErr(w, 400, "验证码错误")
			return
		}
		apiJSON(w, 200, map[string]any{"ok": true})
	}
}

func jsonUnmarshal(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
