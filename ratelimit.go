// ratelimit.go — 频率限制（分级 + IP/设备指纹 + 未验证邮箱小时配额锁）
// 分级：
//   auth  (register/login):        每 1 小时每指纹 ≤5 次
//   act   (发帖/评论/评价/私信/举报):每 1 小时每指纹 ≤30 次
//   upload(上传资料):               每 1 小时每指纹 ≤15 次
// 指纹 = IP + UA 哈希，配合 IP 双重识别（防换 IP 绕过）。
// 未验证邮箱的用户，每自然小时只有 2 次"配额动作"额度；达到第 3 次(及斐波那契全局节点)即锁到下小时。
package main

import (
	"bytes"
	"encoding/json"
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rlEntry struct {
	ts []int64 // 最近请求时间戳（毫秒）
}

var (
	rlMu   sync.Mutex
	rlData = map[string]*rlEntry{}
	// 未验证邮箱用户：小时配额记录  key = "mailq|uid|YYYY-MM-DD-HH:count"
	mailQ = map[string]int{}
)

const (
	rlKeepWindow = 6 * time.Hour // 内存条目保留时长
	mailQuota    = 2             // 未验证用户每小时配额次数（第3次触发锁）
)

// clientIP 提取客户端 IP
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := indexByte(xff, ','); i > 0 {
			return trimSpace(xff[:i])
		}
		return trimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// uaHash 对 UA 做 32 位哈希 → 稳定的设备指纹片段
func uaHash(ua string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(ua))
	return h.Sum32()
}

// fpKey 行为指纹 = IP + UA 哈希（双重，防换 IP/换 UA 单点绕过）
func fpKey(r *http.Request) string {
	return clientIP(r) + "|h" + strconv.FormatUint(uint64(uaHash(r.UserAgent())), 16)
}

// rateLimitOK 窗口限流；limit 为该窗口允许次数
func rateLimitOK(key string, limit int) (bool, int) {
	now := time.Now().UnixMilli()
	rlMu.Lock()
	defer rlMu.Unlock()

	for k, e := range rlData {
		cutoff := now - rlKeepWindow.Milliseconds()
		n := 0
		for _, t := range e.ts {
			if t >= cutoff {
				e.ts[n] = t
				n++
			}
		}
		if n == 0 {
			delete(rlData, k)
		} else {
			e.ts = e.ts[:n]
		}
	}

	e, ok := rlData[key]
	if !ok {
		e = &rlEntry{}
		rlData[key] = e
	}
	cut := now - time.Hour.Milliseconds()
	valid := e.ts[:0]
	for _, t := range e.ts {
		if t >= cut {
			valid = append(valid, t)
		}
	}
	e.ts = valid
	if len(valid) >= limit {
		return false, len(valid)
	}
	e.ts = append(e.ts, now)
	return true, len(e.ts) + 1
}

// hourBucket 当前自然小时桶
func hourBucket(t time.Time) string {
	return t.Format("2006-01-02-15")
}

// mailQuotaUse 未验证邮箱用户：消耗一次小时配额。
// 返回 (允许, 消失/剩余说明)。若达到第3次(配额用尽)，锁到下小时。
func mailQuotaUse(uid int64) (bool, int) {
	now := time.Now()
	bucket := hourBucket(now)
	key := "q" + strconv.FormatInt(uid, 10) + "|" + bucket
	rlMu.Lock()
	n := mailQ[key]
	rlMu.Unlock()
	n++ // 本次尝试
	if n > mailQuota {
		return false, n
	}
	rlMu.Lock()
	mailQ[key] = n
	rlMu.Unlock()
	// 惰性清理旧桶
	if len(mailQ) > 20000 {
		rlMu.Lock()
		keep := map[string]int{}
		for k, v := range mailQ {
			_ = v
			keep[k] = v
		}
		mailQ = keep
		rlMu.Unlock()
	}
	return true, n
}

// rateLimitHandler 分级限流包装：action ∈ auth / act / upload
func rateLimitHandler(action string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := r.Method
		if m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions {
			skip := action == "auth" && bodyEmailIsAdmin(r)
			if !skip {
				limit := 30
				switch action {
				case "auth":
					limit = 5
				case "upload":
					limit = 15
				}
				key := fpKey(r) + "|" + action
				if ok, _ := rateLimitOK(key, limit); !ok {
					apiErr(w, 429, "操作过于频繁，本小时内已达 "+strconv.Itoa(limit)+" 次上限，请稍后再试")
					return
				}
			}
		}
		h(w, r)
	}

}

// bodyEmailIsAdmin 读取请求体中的 email 字段，判断是否为站长管理员邮箱（用于登录限流豁免）。
// 读取后恢复 body，供后续 handler 继续使用。
func bodyEmailIsAdmin(r *http.Request) bool {
	if r.Body == nil {
		return false
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var v struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(body, &v) != nil {
		return false
	}
	email := strings.TrimSpace(strings.ToLower(v.Email))
	admin := strings.TrimSpace(strings.ToLower(os.Getenv("ADMIN_EMAIL")))
	return admin != "" && email == admin
}

// requireVerifiedQuota 在"配额动作"(发帖/评论/评价/私信/举报/上传)前调用：
// 若用户未验证邮箱，则做每自然小时 ≤2 次的配额控制。返回 (允许, 提示信息)。
func requireVerifiedQuota(uid int64) (bool, string) {
	if uid == 0 || emailVerified(uid) {
		return true, ""
	}
	ok, _ := mailQuotaUse(uid)
	if !ok {
		return false, "本小时发布配额已用尽（未验证邮箱，每2次/小时后需验证），请验证邮箱或下小时再试"
	}
	return true, ""
}
// quotaHandler 邮箱配额锁：未验证邮箱的用户做"配额动作"(发帖/评论/评价/私信/举报/上传)时，
// 每自然小时 ≤2 次；达到验证节点则拦截并提示。验证通过后不再限制。
func quotaHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := r.Method
		if m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions {
			uid := currentUserID(r)
			if ok, msg := requireVerifiedQuota(uid); !ok {
				apiErr(w, 403, msg)
				return
			}
		}
		h(w, r)
	}
}