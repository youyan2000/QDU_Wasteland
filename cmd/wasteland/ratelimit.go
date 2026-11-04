// ratelimit.go — 按可信客户端 IP 分级限流 + 未验证邮箱小时配额。
// auth / act / upload：每小时每 IP 分别允许 20 / 30 / 15 次；登录成功后会清零 auth 计数。
// UA 不参与身份键；条目有容量上限，过期记录由生命周期清理器回收。
// 未验证邮箱的用户，每自然小时只有 2 次"配额动作"额度；达到第 3 次(及斐波那契全局节点)即锁到下小时。
package main

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rlEntry struct {
	ts []int64 // 当前小时滑动窗口内的已接受请求，最多 rlMaxEvents 个
}

type mailQuotaEntry struct {
	bucket string
	count  int
}

var (
	rlMu   sync.Mutex
	rlData = map[string]*rlEntry{}
	mailQ  = map[int64]mailQuotaEntry{}
)

const (
	rlKeepWindow = time.Hour
	rlMaxEntries = 50000
	rlMaxEvents  = 120 // 不小于当前最大的验证码小时额度
	mailQuota    = 2
)

// startRateLimitCleanup 在服务生命周期内定期回收；请求路径只检查当前键。
func startRateLimitCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				cleanupRateLimits(now)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func cleanupRateLimits(now time.Time) {
	cut := now.Add(-rlKeepWindow).UnixMilli()
	bucket := hourBucket(now)
	rlMu.Lock()
	defer rlMu.Unlock()
	for key, entry := range rlData {
		if len(entry.ts) == 0 || entry.ts[len(entry.ts)-1] <= cut {
			delete(rlData, key)
		}
	}
	for uid, entry := range mailQ {
		if entry.bucket != bucket {
			delete(mailQ, uid)
		}
	}
}

// clientIP 提取客户端 IP。
//
// 安全约定：只信 r.RemoteAddr。入站的 X-Forwarded-For 完全由客户端决定，
// 任何"取第一段"的实现都会让每个请求落到新的计数桶（=限流失效）。
//
// 唯一例外：当对端就是本机（nginx 反代固定 127.0.0.1）时，采信 nginx 写入的
// X-Real-IP —— 该头由 nginx 覆盖赋值（proxy_set_header X-Real-IP $remote_addr），
// 客户端无法伪造。若将来直接对公网监听，本分支必须去掉。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if isLocalPeer(host) {
		if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
			if normalized := normalizeClientIP(real); normalized != "" {
				return normalized
			}
		}
	}
	if normalized := normalizeClientIP(host); normalized != "" {
		return normalized
	}
	return host
}

func normalizeClientIP(raw string) string {
	ip := net.ParseIP(raw)
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	// IPv6 地址按 /64 聚合，避免同一常规子网轮换临时地址分散额度。
	mask := net.CIDRMask(64, 128)
	return (&net.IPNet{IP: ip.Mask(mask), Mask: mask}).String()
}

// isLocalPeer 判断是否来自本机反代。
func isLocalPeer(host string) bool {
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// fpKey 保留旧调用名，但主键仅使用规范化的可信客户端 IP。
func fpKey(r *http.Request) string {
	return clientIP(r)
}

// clearRateLimit 清空某个键的计数（登录成功后调用，避免正常用户被自己的成功登录锁住）。
func clearRateLimit(key string) {
	rlMu.Lock()
	delete(rlData, key)
	rlMu.Unlock()
}

// rateLimitOK 窗口限流；limit 为该窗口允许次数
func rateLimitOK(key string, limit int) (bool, int) {
	now := time.Now().UnixMilli()
	rlMu.Lock()
	defer rlMu.Unlock()

	if limit <= 0 {
		return false, 0
	}
	if limit > rlMaxEvents {
		limit = rlMaxEvents
	}
	e, ok := rlData[key]
	if !ok {
		// 满容量时不驱逐活跃额度，防止已有额度意外重置。
		if len(rlData) >= rlMaxEntries {
			return false, 0
		}
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
	return true, len(e.ts)
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
	rlMu.Lock()
	defer rlMu.Unlock()
	entry, exists := mailQ[uid]
	if !exists && len(mailQ) >= rlMaxEntries {
		return false, mailQuota + 1
	}
	if entry.bucket != bucket {
		entry = mailQuotaEntry{bucket: bucket}
	}
	if entry.count >= mailQuota {
		return false, entry.count + 1
	}
	entry.count++
	mailQ[uid] = entry
	return true, entry.count
}

// rateLimitHandler 分级限流包装：action ∈ auth / act / upload
func rateLimitHandler(action string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := r.Method
		if m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions {
			limit := 30
			switch action {
			case "auth":
				limit = 20
			case "mail":
				limit = 10 // 邮箱验证/找回密码：每 1 小时每指纹 ≤10 次（与登录配额分离，避免误伤）
			case "upload":
				limit = 15
			}
			key := fpKey(r) + "|" + action
			if ok, _ := rateLimitOK(key, limit); !ok {
				apiErr(w, 429, "操作过于频繁，本小时内已达 "+strconv.Itoa(limit)+" 次上限，请稍后再试")
				return
			}
		}
		h(w, r)
	}

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
