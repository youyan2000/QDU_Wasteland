// mail.go — V12 (B1) 邮件发送模块
// 支持 SMTP（环境变量 MAIL_HOST/PORT/USER/PASS/FROM）；未配置时把邮件内容打印到服务器日志，
// 便于本地验收（验证码/链接直接可见）。
// 提供：生成并存储 email_tokens、发送验证码/找回链接邮件。
package main

import (
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"log"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// ---- token 工具 ----

// genCode 生成 N 位数字验证码（用于邮件正文展示）
func genCode(n int) string {
	const digits = "0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "000000"
	}
	for i := range b {
		b[i] = digits[int(b[i])%len(digits)]
	}
	return string(b)
}

// genToken 生成随机 token（用于链接）
func genToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// makeEmailToken 生成并入库一个 token；type: verify / reset
func makeEmailToken(db *sql.DB, userID int64, typ string) (code, token string, err error) {
	code = genCode(6)
	token = genToken()
	expires := time.Now().Add(30 * time.Minute).UnixMilli()
	_, err = db.Exec(
		"INSERT INTO email_tokens (user_id, type, code, token, expires_at, used, created_at) VALUES (?,?,?,?,?,0,?)",
		userID, typ, code, token, expires, time.Now().Format("2006-01-02 15:04:05"))
	return code, token, err
}

// consumeEmailToken 校验并消费 token；成功返回 userID
func consumeEmailToken(db *sql.DB, token, typ string) int64 {
	var userID int64
	var expires int64
	var used int
	err := db.QueryRow(
		"SELECT user_id, expires_at, used FROM email_tokens WHERE token=? AND type=? ORDER BY id DESC LIMIT 1",
		token, typ).Scan(&userID, &expires, &used)
	if err != nil || used != 0 {
		return 0
	}
	if time.Now().UnixMilli() > expires {
		return 0
	}
	_, _ = db.Exec("UPDATE email_tokens SET used=1 WHERE token=?", token)
	return userID
}

// consumeEmailCode 用 6 位码校验（返回是否有效），码用后作废
func consumeEmailCode(db *sql.DB, userID int64, typ, code string) bool {
	var id int64
	var expires int64
	var used int
	err := db.QueryRow(
		"SELECT id, expires_at, used FROM email_tokens WHERE user_id=? AND type=? AND code=? ORDER BY id DESC LIMIT 1",
		userID, typ, code).Scan(&id, &expires, &used)
	if err != nil || used != 0 {
		return false
	}
	if time.Now().UnixMilli() > expires {
		return false
	}
	_, _ = db.Exec("UPDATE email_tokens SET used=1 WHERE id=?", id)
	return true
}

// ---- 发送 ----

// sendMail 发送邮件；未配置 SMTP 时打印到日志。
func sendMail(to, subject, htmlBody string) bool {
	host := os.Getenv("MAIL_HOST")
	if host == "" {
		log.Printf("📧 [邮件(日志模式)] 收件人:%s 主题:%s\n%s", to, subject, htmlBody)
		return false
	}
	port := os.Getenv("MAIL_PORT")
	if port == "" {
		port = "587"
	}
	user := os.Getenv("MAIL_USER")
	pass := os.Getenv("MAIL_PASS")
	from := os.Getenv("MAIL_FROM")
	if from == "" {
		from = user
	}
	addr := host + ":" + port
	msg := "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: =?UTF-8?B?" + base64Encode([]byte(subject)) + "?=\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n\r\n" +
		htmlBody

	var err error
	if port == "465" {
		err = sendMailSSL(addr, host, user, pass, from, []string{to}, []byte(msg))
	} else {
		err = smtp.SendMail(addr, smtp.PlainAuth("", user, pass, host), from, []string{to}, []byte(msg))
	}
	if err != nil {
		log.Printf("📧 [邮件发送失败] to=%s err=%v", to, err)
		return false
	}
	log.Printf("📧 [邮件已发送] to=%s subject=%s", to, subject)
	return true
}

// sendMailSSL 465 端口：先 TLS 拨号再走 smtp 客户端
func sendMailSSL(addr, host, user, pass, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, InsecureSkipVerify: false})
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer client.Close()
	if err := client.Auth(smtp.PlainAuth("", user, pass, host)); err != nil {
		return err
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, t := range to {
		if err := client.Rcpt(t); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// sendVerifyMail 发送邮箱验证邮件（B2）；返回验证码（日志模式可读取）
func sendVerifyMail(db *sql.DB, to string, userID int64) (string, string) {
	code, token, err := makeEmailToken(db, userID, "verify")
	if err != nil {
		return "", ""
	}
	body := `<h3>QDU Wasteland 邮箱验证</h3>
<p>你好，你的验证码是：<b style="font-size:20px;color:#ff2e88">` + code + `</b></p>
<p>验证码 30 分钟内有效。若未请求请忽略本邮件。</p>`
	sendMail(to, "QDU Wasteland 邮箱验证", body)
	return code, token
}

// sendResetMail 发送找回密码邮件（B3）；返回 token
// baseURL 用于构造重置链接（如 http://localhost:3000）
func sendResetMail(db *sql.DB, to string, userID int64, baseURL string) string {
	_, token, err := makeEmailToken(db, userID, "reset")
	if err != nil {
		return ""
	}
	baseURL = strings.TrimRight(baseURL, "/")
	link := baseURL + "/reset.html?token=" + token
	body := `<h3>QDU Wasteland 找回密码</h3>
<p>点击下面的链接设置新密码（30 分钟内有效）：</p>
<p><a href="` + link + `">` + link + `</a></p>
<p>如非本人操作请忽略。</p>`
	sendMail(to, "QDU Wasteland 找回密码", body)
	return token
}

// base64Encode 简易 Base64
func base64Encode(b []byte) string {
	const tbl = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var sb strings.Builder
	i := 0
	for ; i+3 <= len(b); i += 3 {
		v := int(b[i])<<16 | int(b[i+1])<<8 | int(b[i+2])
		sb.WriteByte(tbl[(v>>18)&63])
		sb.WriteByte(tbl[(v>>12)&63])
		sb.WriteByte(tbl[(v>>6)&63])
		sb.WriteByte(tbl[v&63])
	}
	rem := len(b) - i
	if rem == 1 {
		v := int(b[i]) << 16
		sb.WriteByte(tbl[(v>>18)&63])
		sb.WriteByte(tbl[(v>>12)&63])
		sb.WriteString("==")
	} else if rem == 2 {
		v := int(b[i])<<16 | int(b[i+1])<<8
		sb.WriteByte(tbl[(v>>18)&63])
		sb.WriteByte(tbl[(v>>12)&63])
		sb.WriteByte(tbl[(v>>6)&63])
		sb.WriteString("=")
	}
	return sb.String()
}