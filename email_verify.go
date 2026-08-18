// email_verify.go — V12 (B2) 邮箱验证（斐波那契次数触发）
// 规则：用户「发布内容总数（帖+评论+评价+文章）」落在斐波那契序列(1,2,3,5,8,13…)时，
//       若邮箱未验证则必须验证后才能继续发布。
// 接口：
//   POST /api/email/verify         发送验证码到我的邮箱
//   POST /api/email/verify/confirm body:{code} 提交验证码
package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// fibSet 斐波那契到 300 为止的集合（第 N 次发帖触发验证）
var fibSet = buildFib(300)

func buildFib(max int) map[int]bool {
	m := map[int]bool{1: true, 2: true}
	a, b := 1, 2
	for b <= max {
		m[b] = true
		a, b = b, a+b
	}
	return m
}

// contentCountOf 统计用户发布内容总数（帖+评论+评价+文章；草稿/待复核不计入）
func contentCountOf(uid int64) int {
	n := 0
	authStore.db.QueryRow("SELECT COUNT(*) FROM posts WHERE user_id=? AND status='正常'", uid).Scan(&n)
	var c2, c3, c4 int
	authStore.db.QueryRow("SELECT COUNT(*) FROM comments WHERE user_id=?", uid).Scan(&c2)
	authStore.db.QueryRow("SELECT COUNT(*) FROM reviews WHERE user_id=? AND status='正常'", uid).Scan(&c3)
	authStore.db.QueryRow("SELECT COUNT(*) FROM articles WHERE user_id=? AND status='正常'", uid).Scan(&c4)
	return n + c2 + c3 + c4
}

// emailVerified 查询用户是否已验证
func emailVerified(uid int64) bool {
	var v int
	authStore.db.QueryRow("SELECT email_verified FROM users WHERE id=?", uid).Scan(&v)
	return v == 1
}

// requireEmailVerified 在发布动作前调用：
// 若本次发布后计数 ∈ 斐波那契 且未验证 → 返回 true（需要验证，应拦截）
func requireEmailVerified(uid int64) bool {
	if emailVerified(uid) {
		return false
	}
	next := contentCountOf(uid) + 1 // 本次即将成为第 next 次
	return fibSet[next]
}

// handleRequestEmailVerify 发送验证码到当前登录用户邮箱
func handleRequestEmailVerify() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 {
			apiErr(w, 401, "请先登录")
			return
		}
		var email string
		authStore.db.QueryRow("SELECT email FROM users WHERE id=?", uid).Scan(&email)
		if email == "" {
			apiErr(w, 500, "读取邮箱失败")
			return
		}
		code, _ := sendVerifyMail(authStore.db, email, uid)
		if code == "" {
			apiErr(w, 500, "验证码生成失败")
			return
		}
		apiJSON(w, 200, map[string]any{"message": "验证码已发送到邮箱（未配置SMTP时见服务器日志）"})
	}
}

// handleConfirmEmailVerify 提交验证码
func handleConfirmEmailVerify() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 {
			apiErr(w, 401, "请先登录")
			return
		}
		var body struct{ Code string `json:"code"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		if !consumeEmailCode(authStore.db, uid, "verify", body.Code) {
			apiErr(w, 400, "验证码错误或已过期")
			return
		}
		_, _ = authStore.db.Exec("UPDATE users SET email_verified=1 WHERE id=?", uid)
		apiJSON(w, 200, map[string]any{"message": "邮箱验证成功", "verified": true})
	}
}
// ---- B3 找回密码 ----

// handleRequestReset 发送重置邮件
// POST /api/email/reset  body: {"email":"..."}
func handleRequestReset() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Email string `json:"email"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		email := strings.ToLower(strings.TrimSpace(body.Email))
		if email == "" {
			apiErr(w, 400, "请输入邮箱")
			return
		}
		var uid int64
		authStore.db.QueryRow("SELECT id FROM users WHERE email=?", email).Scan(&uid)
		if uid == 0 {
			// 不暴露邮箱是否存在：统一返回成功
			apiJSON(w, 200, map[string]string{"message": "如果该邮箱已注册，重置链接已发送"})
			return
		}
		// 用实际请求的 host 构造链接（避免硬编码 localhost 导致生产/换端口失效）
		base := "http://" + r.Host
		sendResetMail(authStore.db, email, uid, base)
		apiJSON(w, 200, map[string]string{"message": "如果该邮箱已注册，重置链接已发送"})
	}
}

// handleConfirmReset 通过 token 重置密码
// POST /api/email/reset/confirm  body: {"token":"...","password":"..."}
func handleConfirmReset() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Token    string `json:"token"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		body.Token = strings.TrimSpace(body.Token)
		if len(body.Password) < 6 {
			apiErr(w, 400, "密码至少 6 位")
			return
		}
		uid := consumeEmailToken(authStore.db, body.Token, "reset")
		if uid == 0 {
			apiErr(w, 400, "链接无效或已过期")
			return
		}
		hash, err := bcryptHash(body.Password)
		if err != nil {
			apiErr(w, 500, "密码处理失败")
			return
		}
		_, err = authStore.db.Exec("UPDATE users SET password=? WHERE id=?", hash, uid)
		if err != nil {
			apiErr(w, 500, "重置失败")
			return
		}
		apiJSON(w, 200, map[string]string{"message": "密码已重置，请用新密码登录"})
	}
}