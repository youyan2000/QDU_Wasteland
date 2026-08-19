// auth_http.go — 认证 HTTP 接口与会话 (V2)
package main

import (
	"crypto/rand"
	"io"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ---- 服务端会话存储 ----
var (
	sessions   = map[string]int64{} // sessionID -> userID
	sessionsMu sync.Mutex
)

const sessionCookie = "qdu_session"

func newSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// createSession 创建会话并生成 Cookie
func createSession(w http.ResponseWriter, userID int64) {
	id := newSessionID()
	sessionsMu.Lock()
	sessions[id] = userID
	sessionsMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		MaxAge:   7 * 24 * 3600, // 7 天
		HttpOnly: true,          // 防 XSS 读取
		Secure:   true,          // 仅 HTTPS 传输（已上线 HTTPS）
		SameSite: http.SameSiteStrictMode, // A3 CSRF：跨站非 GET 请求不带会话 Cookie
	})
}

// currentUserID 从请求读会话，返回 userID
func currentUserID(r *http.Request) int64 {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return 0
	}
	sessionsMu.Lock()
	id := sessions[c.Value]
	sessionsMu.Unlock()
	return id
}

// destroySession 清除会话
func destroySession(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(sessionCookie)
	if err == nil {
		sessionsMu.Lock()
		delete(sessions, c.Value)
		sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true,
	})
}

// ---- 工具 ----
var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func apiErr(w http.ResponseWriter, status int, msg string) {
	apiJSON(w, status, map[string]string{"error": msg})
}

// ---- 已登录通用的当前用户辅助 ----
func handleMe(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 {
			apiJSON(w, 200, map[string]any{"loggedIn": false})
			return
		}
		u, err := auth.GetByID(uid)
		if err != nil || u == nil {
			destroySession(w, r)
			apiJSON(w, 200, map[string]any{"loggedIn": false})
			return
		}
		si := u.ToSessionInfo()
		si.Level, si.LevelTitle = computeUserLevel(auth, uid)
		apiJSON(w, 200, map[string]any{"loggedIn": true, "user": si})
	}
}

// computeUserLevel 按累计贡献（帖子+评价+资料+文章）算等级
func computeUserLevel(a *AuthStore, uid int64) (int, string) {
	var p, r, f, ar int
	a.db.QueryRow("SELECT COUNT(*) FROM posts WHERE user_id=? AND status='正常'", uid).Scan(&p)
	a.db.QueryRow("SELECT COUNT(*) FROM reviews WHERE user_id=?", uid).Scan(&r)
	a.db.QueryRow("SELECT COUNT(*) FROM files WHERE uploader_id=?", uid).Scan(&f)
	a.db.QueryRow("SELECT COUNT(*) FROM articles WHERE user_id=? AND status='正常'", uid).Scan(&ar)
	pts := p + r + f + ar
	switch {
	case pts >= 100:
		return 5, "传奇学长"
	case pts >= 50:
		return 4, "资深战友"
	case pts >= 20:
		return 3, "校园达人"
	case pts >= 5:
		return 2, "活跃同学"
	default:
		return 1, "新同学"
	}
}

// ---- 注册 ----
type registerReq struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
	College  string `json:"college"`
	Major    string `json:"major"`
	Captcha  string `json:"captcha"`
	CaptchaID string `json:"captchaId"`
}

func handleRegister(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		req.Username = strings.TrimSpace(strings.ToLower(req.Username))
		req.Nickname = strings.TrimSpace(req.Nickname)
		req.College = strings.TrimSpace(req.College)
		req.Major = strings.TrimSpace(req.Major)

		// 图形验证码 (防脚本批量注册)
		if !verifyCaptchaCode(req.CaptchaID, req.Captcha) {
			apiErr(w, 400, "验证码错误，请重试")
			return
		}

		if req.Email == "" || !emailRe.MatchString(req.Email) {
			apiErr(w, 400, "请输入有效的邮箱")
			return
		}
		if len(req.Password) < 6 {
			apiErr(w, 400, "密码至少 6 位")
			return
		}
		if req.Username == "" {
			apiErr(w, 400, "请设置用户名（用于登录）")
			return
		}
		if len(req.Username) < 2 || len(req.Username) > 20 {
			apiErr(w, 400, "用户名需 2~20 个字符")
			return
		}
		// 昵称=用户名（合并为一个，全站唯一）：昵称未填时用用户名
		if req.Nickname == "" {
			req.Nickname = req.Username
		}
		// 学院和专业必填（按你的产品要求）
		if req.College == "" || req.Major == "" {
			apiErr(w, 400, "请填写学院和专业")
			return
		}
		// J2：学院/专业必须从统一清单选择，存规范名
		req.College = canonicalCollege(req.College)
		collegeOK := false
		for _, c := range orgColleges {
			if c == req.College {
				collegeOK = true
				break
			}
		}
		if !collegeOK {
			apiErr(w, 400, "请从列表中选择有效的学院")
			return
		}
		majors := orgMajors[req.College]
		majorOK := false
		for _, m := range majors {
			if m == req.Major {
				majorOK = true
				break
			}
		}
		if !majorOK {
			apiErr(w, 400, "请从列表中选择该学院下的专业")
			return
		}

		exist, _ := auth.GetByEmail(req.Email)
		if exist != nil {
			apiErr(w, 409, "该邮箱已被注册")
			return
		}

		// 用户名唯一校验
		var dupU int
		auth.db.QueryRow("SELECT COUNT(*) FROM users WHERE username=?", req.Username).Scan(&dupU)
		if dupU > 0 {
			apiErr(w, 409, "该用户名已被占用")
			return
		}
		u, err := auth.CreateUser(req.Email, req.Username, req.Password, req.Nickname, req.College, req.Major)
		if err != nil {
			apiErr(w, 500, "注册失败")
			return
		}
		createSession(w, u.ID)
		apiJSON(w, 200, map[string]any{"message": "注册成功已登录", "user": u.ToSessionInfo()})
	}
}

// ---- 登录 ----
type loginReq struct {
	Email    string `json:"email"`
	Account  string `json:"account"` // E1: 用户名或邮箱
	Password string `json:"password"`
	Captcha  string `json:"captcha"`
	CaptchaID string `json:"captchaId"`
}

func handleLogin(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apiErr(w, 400, "请求格式错误")
			return
		}
		// 验证码校验（防暴力破解）
		if !verifyCaptchaCode(strings.TrimSpace(req.CaptchaID), strings.TrimSpace(req.Captcha)) {
			apiErr(w, 423, "验证码错误或已过期，请刷新后重试")
			return
		}
		inp := strings.ToLower(strings.TrimSpace(req.Email))
		if inp == "" {
			inp = strings.ToLower(strings.TrimSpace(req.Account))
		}
		if inp == "" {
			apiErr(w, 400, "请输入用户名或邮箱")
			return
		}
		// E1: 含@按邮箱查，否则按用户名查（默认用户名登录）
		var u *User
		if emailRe.MatchString(inp) {
			u, _ = auth.GetByEmail(inp)
		} else {
			u, _ = auth.GetByUsername(inp)
		}
		if u == nil || !auth.VerifyPassword(u.Password, req.Password) {
			apiErr(w, 401, "账号或密码错误")
			return
		}
		if u.Banned == 1 {
			apiErr(w, 403, "该账号已被封禁，无法登录")
			return
		}
		createSession(w, u.ID)
		apiJSON(w, 200, map[string]any{"message": "登录成功", "user": u.ToSessionInfo()})
	}
}

// ---- 退出 ----
func handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		destroySession(w, r)
		apiJSON(w, 200, map[string]any{"message": "已退出"})
	}
}

// ---- 会话清理（可选长期）----
func init() {
	// 定期清理过期会话（简单实现，V2 不做自动过期扫描，依赖 Cookie MaxAge）
	_ = sync.Once{}
	_ = time.Now()
}

// handleUpdateProfile 更新个人资料（性别/年龄/籍贯/个性签名/学院/专业）
// POST /api/me/profile
func handleUpdateProfile(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 { apiErr(w, 401, "请先登录"); return }
		var body struct {
			Nickname    string         `json:"nickname"`
			Gender      string         `json:"gender"`
			Age         int            `json:"age"`
			Birthday    string         `json:"birthday"`
			Grade       string         `json:"grade"`
			NativePlace string         `json:"nativePlace"`
			Wechat      string         `json:"wechat"`
			QQ          string         `json:"qq"`
			Phone       string         `json:"phone"`
			Social      string         `json:"social"`
			Bio         string         `json:"bio"`
			College     string         `json:"college"`
			Major       string         `json:"major"`
			Privacy     map[string]any `json:"privacy"`
		}
		rawBody, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(rawBody, &body); err != nil {
			apiErr(w, 400, "请求格式错误"); return
		}
		// 检测本次请求是否提交了任何个人资料字段（隐私设置单独保存时不得触碰资料）
		hasProfile := false
		{
			var raw map[string]json.RawMessage
			_ = json.Unmarshal(rawBody, &raw)
			for _, k := range []string{"nickname", "gender", "age", "birthday", "grade", "nativePlace", "wechat", "qq", "phone", "social", "bio", "college", "major"} {
				if _, ok := raw[k]; ok { hasProfile = true; break }
			}
		}
		if !hasProfile {
			// 仅隐私设置：只更新 privacy 列，其他资料原样保留
			if len(body.Privacy) > 0 {
				privacyJSON, _ := json.Marshal(body.Privacy)
				_, _ = auth.db.Exec("UPDATE users SET privacy=? WHERE id=?", string(privacyJSON), uid)
			}
			u, _ := auth.GetByID(uid)
			apiJSON(w, 200, map[string]any{"message": "已保存", "user": u.ToSessionInfo()})
			return
		}
		// 后端输入校验（不可信输入必须双重校验）
		body.Nickname = cleanUserText(body.Nickname, 20)
		if body.Nickname != "" {
			if n := utf8.RuneCountInString(body.Nickname); n < 2 || n > 20 {
				apiErr(w, 400, "昵称需 2~20 个字符"); return
			}
			// 昵称全站唯一（注册时昵称=用户名，编辑时也保持唯一，排除自己）
			var dup int
			auth.db.QueryRow(`SELECT COUNT(*) FROM users WHERE nickname=? AND id<>?`, body.Nickname, uid).Scan(&dup)
			if dup > 0 { apiErr(w, 409, "该昵称已被占用"); return }
		}
		if body.Age < 1 || body.Age > 100 { body.Age = 0 }
		body.NativePlace = cleanUserText(body.NativePlace, 100)
		body.Bio = cleanUserText(body.Bio, 20)
		body.Gender = cleanUserText(body.Gender, 6)
		body.College = cleanUserText(body.College, 40)
		body.Major = cleanUserText(body.Major, 40)
		body.Birthday = cleanUserText(body.Birthday, 20)
		body.Grade = cleanUserText(body.Grade, 10)
		body.Wechat = cleanUserText(body.Wechat, 40)
		body.QQ = cleanUserText(body.QQ, 20)
		body.Phone = cleanUserText(body.Phone, 20)
		body.Social = cleanUserText(body.Social, 120)
		// 格式校验（非空才校验）
		if body.QQ != "" {
			if !isAllDigits(body.QQ) || len(body.QQ) < 5 || len(body.QQ) > 12 {
				apiErr(w, 400, "QQ 号应为 5~12 位数字"); return
			}
		}
		if body.Phone != "" {
			if !isAllDigits(body.Phone) || len(body.Phone) < 7 || len(body.Phone) > 15 {
				apiErr(w, 400, "电话号码应为 7~15 位数字"); return
			}
		}
		if body.Grade != "" {
			if !isGradeFormat(body.Grade) {
				apiErr(w, 400, "届别格式应为如 2023级（4位年份+级）"); return
			}
		}
		// J5：籍贯必须是"省份 市"组合（前端下拉 + 后端校验）
		if body.NativePlace != "" && !isValidNativePlace(body.NativePlace) {
			apiErr(w, 400, "籍贯请从省、市下拉中选择"); return
		}
		// J2：学院/专业若在编辑资料时被提交，须从统一清单选择
		if body.College != "" || body.Major != "" {
			body.College = canonicalCollege(body.College)
			collegeOK := false
			for _, c := range orgColleges {
				if c == body.College { collegeOK = true; break }
			}
			if !collegeOK {
				apiErr(w, 400, "请从列表中选择有效的学院"); return
			}
			if body.Major == "" {
				apiErr(w, 400, "请选择专业"); return
			}
			majorOK := false
			for _, m := range orgMajors[body.College] {
				if m == body.Major { majorOK = true; break }
			}
			if !majorOK {
				apiErr(w, 400, "请从列表中选择该学院下的专业"); return
			}
		}
		// privacy 设置（如需更新）
		if len(body.Privacy) > 0 {
			privacyJSON, _ := json.Marshal(body.Privacy)
			_, _ = auth.db.Exec("UPDATE users SET privacy=? WHERE id=?", string(privacyJSON), uid)
		}
		_, err := auth.db.Exec("UPDATE users SET nickname=?, gender=?, age=?, birthday=?, grade=?, native_place=?, wechat=?, qq=?, phone=?, social=?, bio=?, college=?, major=? WHERE id=?",
			body.Nickname, body.Gender, body.Age, body.Birthday, body.Grade, body.NativePlace, body.Wechat, body.QQ, body.Phone, body.Social, body.Bio, body.College, body.Major, uid)
		if err != nil { apiErr(w, 500, "保存失败"); return }
		u, _ := auth.GetByID(uid)
		apiJSON(w, 200, map[string]any{"message": "已保存", "user": u.ToSessionInfo()})
	}
}

// cleanUserText 清洗用户文本：去除危险字符 + 限制 rune 长度
func cleanUserText(s string, maxLen int) string {
	if s == "" { return "" }
	// 去 HTML/脚本危险字符
	cleaned := strings.NewReplacer("<", "", ">", "", "\\", "", "`", "", ";", "").Replace(s)
	runes := []rune(strings.TrimSpace(cleaned))
	if len(runes) > maxLen { runes = runes[:maxLen] }
	return string(runes)
}



// isAllDigits 判断字符串是否全为数字
func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// isGradeFormat 判断届别格式：4位年份 + "级"（如 2023级）
func isGradeFormat(s string) bool {
	runes := []rune(s)
	if len(runes) != 5 {
		return false
	}
	for i := 0; i < 4; i++ {
		if runes[i] < '0' || runes[i] > '9' {
			return false
		}
	}
	return runes[4] == '级'
}

// handleChangePassword 修改密码（登录状态下）
// POST /api/me/password  body:{oldPassword, newPassword}
func handleChangePassword(auth *AuthStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := currentUserID(r)
		if uid == 0 { apiErr(w, 401, "请先登录"); return }
		var body struct {
			OldPassword string `json:"oldPassword"`
			NewPassword string `json:"newPassword"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiErr(w, 400, "请求格式错误"); return
		}
		if len(body.NewPassword) < 6 {
			apiErr(w, 400, "新密码至少 6 位"); return
		}
		u, err := auth.GetByID(uid)
		if err != nil || u == nil { apiErr(w, 500, "用户不存在"); return }
		// 安全：修改密码前要求邮箱已验证（防止账号被盗后改密）
		if u.EmailVerified != 1 {
			apiErr(w, 403, "请先验证邮箱后再修改密码（可在个人中心发送验证码）"); return
		}
		if !auth.VerifyPassword(u.Password, body.OldPassword) {
			apiErr(w, 400, "旧密码不正确"); return
		}
		hash, err := bcryptHash(body.NewPassword)
		if err != nil { apiErr(w, 500, "密码处理失败"); return }
		_, err = auth.db.Exec("UPDATE users SET password=? WHERE id=?", hash, uid)
		if err != nil { apiErr(w, 500, "修改失败"); return }
		logAudit(auth.db, uid, "修改密码", "用户ID="+strconv.FormatInt(uid, 10))
		apiJSON(w, 200, map[string]string{"message": "密码已修改"})
	}
}