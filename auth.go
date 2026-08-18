// auth.go — 用户认证模块 (V2)
// 用 SQLite 存用户，bcrypt 存密码哈希，Cookie 维持会话。
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"time"

	_ "modernc.org/sqlite" // 纯 Go 的 SQLite 驱动（免 CGO 编译）
	"golang.org/x/crypto/bcrypt"
)

// User 用户记录（不含密码哈希的敏感展示除外，这里保留哈希用于校验）
type User struct {
	ID        int64
	Email     string
	Username  string
	Password  string // 只用于内存校验，不对外输出
	Nickname  string
	College   string // 学院
	Major     string // 专业
	IsAdmin       int    // 是否管理员(0/1)
	EmailVerified int    // 邮箱是否已验证(0/1)
	Gender        string // 性别
	Age           int    // 年龄
	NativePlace   string // 籍贯
	Bio           string // 个性签名
	Avatar        string // 头像 URL（空 = 默认）
	Banned        int    // 是否封禁(0/1)
	Privacy       string // 隐私设置 JSON
	CreatedAt     string
}

// SessionInfo 当前登录用户信息（对外输出，不含密码）
type SessionInfo struct {
	ID            int64  `json:"id"`
	Email         string `json:"email"`
	Username      string `json:"username"`
	Nickname      string `json:"nickname"`
	College       string `json:"college"`
	Major         string `json:"major"`
	IsAdmin       int    `json:"isAdmin"`
	EmailVerified int    `json:"emailVerified"`
	Gender        string `json:"gender"`
	Age           int    `json:"age"`
	NativePlace   string `json:"nativePlace"`
	Bio           string `json:"bio"`
	Avatar        string `json:"avatar,omitempty"`
	Banned        int    `json:"banned"`
	CreatedAt     string `json:"createdAt,omitempty"`
	Level         int    `json:"level"`
	LevelTitle    string `json:"levelTitle"`
	Privacy       map[string]any `json:"privacy"`
}

// AuthStore 认证存储（封装 SQLite）
type AuthStore struct {
	db *sql.DB
}

// openAuthStore 打开/初始化用户数据库
func openAuthStore() (*AuthStore, error) {
	// 数据库文件路径
	dbPath := "qdu-auth.db"
	if os.Getenv("AUTH_DB") != "" {
		dbPath = os.Getenv("AUTH_DB")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	// SQLite 单连接串行化所有访问，避免并发写死锁。
	// 前提：所有 handler 必须正确 Close rows（已逐一核查 + defer）。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=10000")
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		email      TEXT NOT NULL UNIQUE,
		email_verified INTEGER NOT NULL DEFAULT 0,
		password   TEXT NOT NULL,
		nickname   TEXT NOT NULL DEFAULT '',
		college    TEXT NOT NULL DEFAULT '',
		major      TEXT NOT NULL DEFAULT '',
		is_admin   INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS files (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		course_code TEXT NOT NULL,
		title      TEXT NOT NULL DEFAULT '',
		category   TEXT NOT NULL DEFAULT '',
		teacher    TEXT NOT NULL DEFAULT '',
		semester   TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		file_name  TEXT NOT NULL,
		stored_name TEXT NOT NULL,
		size       INTEGER NOT NULL DEFAULT 0,
		uploader_id INTEGER NOT NULL DEFAULT 0,
		is_anonymous INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS reviews (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		course_code TEXT NOT NULL,
		user_id    INTEGER NOT NULL,
		rating     INTEGER NOT NULL DEFAULT 0,
		content    TEXT NOT NULL DEFAULT '',
		is_anonymous INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS forums (
		id    INTEGER PRIMARY KEY,
		slug  TEXT NOT NULL UNIQUE,
		name  TEXT NOT NULL,
		desc  TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS posts (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		forum      TEXT NOT NULL,
		user_id    INTEGER NOT NULL,
		title      TEXT NOT NULL,
		content    TEXT NOT NULL DEFAULT '',
		is_anonymous INTEGER NOT NULL DEFAULT 0,
		likes      INTEGER NOT NULL DEFAULT 0,
		status     TEXT NOT NULL DEFAULT '正常',
		created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS comments (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		post_id    INTEGER NOT NULL,
		user_id    INTEGER NOT NULL,
		content    TEXT NOT NULL DEFAULT '',
		is_anonymous INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS post_likes (
		post_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		PRIMARY KEY (post_id, user_id)
	);
	CREATE TABLE IF NOT EXISTS reports (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		target_type TEXT NOT NULL DEFAULT '',
		target_id   TEXT NOT NULL DEFAULT '',
		target_author_id INTEGER NOT NULL DEFAULT 0,
		reporter_id INTEGER NOT NULL DEFAULT 0,
		reason      TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL DEFAULT '待处理',
		resolved_by INTEGER NOT NULL DEFAULT 0,
		created_at  TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS messages (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		from_id    INTEGER NOT NULL,
		to_id      INTEGER NOT NULL,
		content    TEXT NOT NULL DEFAULT '',
		is_read    INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS notifications (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		recipient_id INTEGER NOT NULL,
		actor_id   INTEGER NOT NULL DEFAULT 0,
		type       TEXT NOT NULL DEFAULT '',
		ref_id     INTEGER NOT NULL DEFAULT 0,
		text       TEXT NOT NULL DEFAULT '',
		is_read    INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS announcements (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		content    TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS email_tokens (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id    INTEGER NOT NULL,
		type       TEXT NOT NULL DEFAULT 'verify',   -- verify=邮箱验证 reset=找回密码
		code       TEXT NOT NULL,                     -- 6位数字验证码(邮件内展示)
		token      TEXT NOT NULL,                     -- 链接token
		expires_at INTEGER NOT NULL,                  -- 过期时间戳(毫秒)
		used       INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS articles (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		title      TEXT NOT NULL,
		category   TEXT NOT NULL DEFAULT '',
		content    TEXT NOT NULL DEFAULT '',
		user_id    INTEGER NOT NULL DEFAULT 0,
		is_anonymous INTEGER NOT NULL DEFAULT 0,
		status     TEXT NOT NULL DEFAULT '正常',
		created_at TEXT NOT NULL
	)`)
	if err != nil {
		return nil, err
	}
	// 迁移：给旧 files 表补充新列（若不存在）
	if err := migrateFilesTable(db); err != nil {
		return nil, err
	}
	// 迁移：users 表补充 is_admin 列（若不存在）
	if err := migrateUsersTable(db); err != nil {
		return nil, err
	}
	// 迁移：reviews 表补充 status 列（若不存在）
	if err := migrateReviewsTable(db); err != nil {
		return nil, err
	}
	// 迁移：posts 表补充 status 列 (C1)
	if err := migratePostsStatus(db); err != nil {
		return nil, err
	}
	// 文章旧状态归一 (C1)
	migrateArticleStatus(db)
	// 投稿流：文章表补 reject_reason 列 (F3)
	if err := migrateArticleRejectReason(db); err != nil {
		return nil, err
	}
	// 评论楼中楼：补 parent_id 列 (G1)
	if err := migrateCommentsParentId(db); err != nil {
		return nil, err
	}
	// 用户主页隐私：补 privacy 列 (G3)
	if err := migrateUsersPrivacy(db); err != nil {
		return nil, err
	}
	// 种子：四个论坛广场
	seedForums(db)
	return &AuthStore{db: db}, nil
}

// seedForums 插入默认四广场（幂等）
func seedForums(db *sql.DB) {
	forums := []struct{ slug, name, desc string }{
		{"trade", "交易广场", "二手书、闲置物品买卖置换"},
		{"paper", "纸片广场", "吐槽、树洞、分享情绪与观点"},
		{"help", "求助广场", "求资料、找人脉、问经验"},
		{"friend", "友人广场", "失物招领、拼车拼饭、相约出行"},
	}
	for _, f := range forums {
		var cnt int
		db.QueryRow("SELECT COUNT(*) FROM forums WHERE slug = ?", f.slug).Scan(&cnt)
		if cnt == 0 {
			db.Exec("INSERT INTO forums (slug,name,desc) VALUES (?,?,?)", f.slug, f.name, f.desc)
		}
	}
}

// migrateFilesTable 确保 files 表包含 V3 新增列
func migrateFilesTable(db *sql.DB) error {
	cols := map[string]string{
		"category":    "TEXT NOT NULL DEFAULT ''",
		"teacher":     "TEXT NOT NULL DEFAULT ''",
		"semester":    "TEXT NOT NULL DEFAULT ''",
		"description": "TEXT NOT NULL DEFAULT ''",
		"status":      "TEXT NOT NULL DEFAULT '正常'",
	}
	for col, def := range cols {
		var cnt int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('files') WHERE name=?`, col).Scan(&cnt); err != nil {
			return err
		}
		if cnt == 0 {
			if _, err := db.Exec("ALTER TABLE files ADD COLUMN " + col + " " + def); err != nil {
				return err
			}
		}
	}
	return nil
}

// migratePostsStatus 确保 posts 表包含 status 列 (C1 草稿箱)
func migratePostsStatus(db *sql.DB) error {
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('posts') WHERE name='status'`).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		_, err := db.Exec("ALTER TABLE posts ADD COLUMN status TEXT NOT NULL DEFAULT '正常'")
		return err
	}
	return nil
}

// migrateArticleStatus 文章旧状态归一：已下架 → draft (C1)
func migrateArticleStatus(db *sql.DB) {
	_, _ = db.Exec("UPDATE articles SET status='draft' WHERE status NOT IN ('正常','draft','待复核','待审','已驳回')")
}

// migrateReviewsTable 确保 reviews 表包含 status 列 (V7 评价后台)
func migrateReviewsTable(db *sql.DB) error {
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('reviews') WHERE name='status'`).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		_, err := db.Exec(`ALTER TABLE reviews ADD COLUMN status TEXT NOT NULL DEFAULT '正常'`)
		return err
	}
	return nil
}
// migrateArticleRejectReason 确保 articles 表包含 reject_reason 列 (F3 投稿驳回理由)
func migrateArticleRejectReason(db *sql.DB) error {
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('articles') WHERE name='reject_reason'`).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		_, err := db.Exec(`ALTER TABLE articles ADD COLUMN reject_reason TEXT NOT NULL DEFAULT ''`)
		return err
	}
	return nil
}

// migrateUsersTable 确保 users 表包含 is_admin 列
func migrateUsersTable(db *sql.DB) error {
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='is_admin'`).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		_, err := db.Exec("ALTER TABLE users ADD COLUMN is_admin INTEGER NOT NULL DEFAULT 0")
		return err
	}
	// email_verified (B2)
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='email_verified'`).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		_, err := db.Exec("ALTER TABLE users ADD COLUMN email_verified INTEGER NOT NULL DEFAULT 0")
		return err
	}
	// 存量用户视为已验证（避免老账号被邮箱验证卡住；新注册用户默认未验证）
	_, _ = db.Exec("UPDATE users SET email_verified=1 WHERE email_verified=0")
	// 个人资料：性别/年龄/籍贯 (V14)
	for _, col := range []string{"gender", "age", "native_place", "bio"} {
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='`+col+`'`).Scan(&cnt); err != nil {
			return err
		}
		if cnt == 0 {
			def := "TEXT NOT NULL DEFAULT ''"
			if col == "age" { def = "INTEGER NOT NULL DEFAULT 0" }
			if _, err := db.Exec("ALTER TABLE users ADD COLUMN " + col + " " + def); err != nil {
				return err
			}
		}
	}
	// banned (F2 封禁)
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='banned'`).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		if _, err := db.Exec("ALTER TABLE users ADD COLUMN banned INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	// username (E1)
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='username'`).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		if _, err := db.Exec("ALTER TABLE users ADD COLUMN username TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
		// 存量用户回填 username = email@前部分（唯一性由前端防重 + email先得）
		_, _ = db.Exec("UPDATE users SET username = lower(substr(email,1,instr(email,'@')-1)) WHERE username=''")
	}
	// avatar (头像，空 = 默认)
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='avatar'`).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		if _, err := db.Exec("ALTER TABLE users ADD COLUMN avatar TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	return nil
}

// CreateUser 注册新用户
func (s *AuthStore) CreateUser(email, username, password, nickname, college, major string) (*User, error) {
	// 密码哈希（cost=10）
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return nil, err
	}
	created := time.Now().Format("2006-01-02 15:04:05")
	res, err := s.db.Exec(
		"INSERT INTO users (email, username, password, nickname, college, major, created_at) VALUES (?,?,?,?,?,?,?)",
		email, username, string(hash), nickname, college, major, created,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetByID(id)
}

// GetByEmail 按邮箱查用户
func (s *AuthStore) GetByEmail(email string) (*User, error) {
	row := s.db.QueryRow(
		"SELECT id, email, username, password, nickname, college, major, is_admin, email_verified, gender, age, native_place, bio, avatar, banned, privacy, created_at FROM users WHERE email = ?",
		email)
	u := &User{}
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.Password, &u.Nickname, &u.College, &u.Major, &u.IsAdmin, &u.EmailVerified, &u.Gender, &u.Age, &u.NativePlace, &u.Bio, &u.Avatar, &u.Banned, &u.Privacy, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

// GetByUsername 按用户名查用户 (E1)
func (s *AuthStore) GetByUsername(username string) (*User, error) {
	row := s.db.QueryRow("SELECT id, email, username, password, nickname, college, major, is_admin, email_verified, gender, age, native_place, bio, avatar, banned, privacy, created_at FROM users WHERE username = ?", username)
	u := &User{}
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.Password, &u.Nickname, &u.College, &u.Major, &u.IsAdmin, &u.EmailVerified, &u.Gender, &u.Age, &u.NativePlace, &u.Bio, &u.Avatar, &u.Banned, &u.Privacy, &u.CreatedAt)
	if err != nil {
		return nil, nil
	}
	return u, nil
}

// GetByID 按 ID 查用户
func (s *AuthStore) GetByID(id int64) (*User, error) {
	row := s.db.QueryRow(
		"SELECT id, email, username, password, nickname, college, major, is_admin, email_verified, gender, age, native_place, bio, avatar, banned, privacy, created_at FROM users WHERE id = ?",
		id)
	u := &User{}
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.Password, &u.Nickname, &u.College, &u.Major, &u.IsAdmin, &u.EmailVerified, &u.Gender, &u.Age, &u.NativePlace, &u.Bio, &u.Avatar, &u.Banned, &u.Privacy, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

// GetAllUsers 返回所有用户（用于后台用户管理，不含密码哈希）
func (s *AuthStore) GetAllUsers() ([]SessionInfo, error) {
	rows, err := s.db.Query(
		"SELECT id, email, nickname, college, major, is_admin, banned, created_at FROM users ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionInfo{}
	for rows.Next() {
		si := SessionInfo{}
		var created string
		if err := rows.Scan(&si.ID, &si.Email, &si.Nickname, &si.College, &si.Major, &si.IsAdmin, &si.Banned, &created); err != nil {
			return nil, err
		}
		si.CreatedAt = created
		out = append(out, si)
	}
	return out, rows.Err()
}

// SetAdmin 设置某用户是否为管理员
func (s *AuthStore) SetAdmin(id int64, isAdmin int) error {
	_, err := s.db.Exec("UPDATE users SET is_admin = ? WHERE id = ?", isAdmin, id)
	return err
}

// VerifyPassword 校验密码
func (s *AuthStore) VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// ToSessionInfo 转对外信息
func (u *User) ToSessionInfo() SessionInfo {
	priv := map[string]any{"age": 0, "gender": 0, "college": 1, "major": 1, "nativePlace": 1}
	if u.Privacy != "" {
		if err := json.Unmarshal([]byte(u.Privacy), &priv); err != nil {
			priv = map[string]any{"age": 0, "gender": 0, "college": 1, "major": 1, "nativePlace": 1}
		}
	}
	return SessionInfo{
		ID: u.ID, Email: u.Email, Username: u.Username, Nickname: u.Nickname,
		College: u.College, Major: u.Major, IsAdmin: u.IsAdmin,
		EmailVerified: u.EmailVerified, Gender: u.Gender, Age: u.Age,
		NativePlace: u.NativePlace, Bio: u.Bio, Banned: u.Banned,
		CreatedAt: u.CreatedAt,
		Privacy: priv,
	}
}

// Close 关闭数据库
func (s *AuthStore) Close() {
	if s.db != nil {
		s.db.Close()
	}
}

// initAuth 便捷初始化（main 里调用）
var authStore *AuthStore

func initAuth() error {
	st, err := openAuthStore()
	if err != nil {
		return err
	}
	authStore = st
	// 关键词自动审查：建表 + 种子默认词库 + 加载内存词表 (V8)
	initCensor(st.db)
	log.Println("✅ 用户数据库已连接 (qdu-auth.db) 敏感词 " + strconv.Itoa(len(censorWords)) + " 条")
	return nil
}

// bcryptHash 生成密码哈希（供重置密码等复用）
func bcryptHash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// migrateCommentsParentId 确保 comments 表含 parent_id 列（G1 楼中楼）
func migrateCommentsParentId(db *sql.DB) error {
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('comments') WHERE name='parent_id'`).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		_, err := db.Exec(`ALTER TABLE comments ADD COLUMN parent_id INTEGER NOT NULL DEFAULT 0`)
		return err
	}
	return nil
}

// migrateUsersPrivacy 确保 users 表含 privacy 列（存 JSON 隐私偏好）
func migrateUsersPrivacy(db *sql.DB) error {
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='privacy'`).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		_, err := db.Exec(`ALTER TABLE users ADD COLUMN privacy TEXT NOT NULL DEFAULT '{"age":0,"gender":0,"college":1,"major":1,"nativePlace":1}'`)
		return err
	}
	return nil
}

// getUserPrivacy 读取用户隐私 JSON，解析失败返回默认（仅学院/专业公开）
func getUserPrivacy(sdb *sql.DB, uid int64) map[string]any {
	privacy := map[string]any{"age": 0, "gender": 0, "college": 1, "major": 1, "nativePlace": 1}
	var raw string
	_ = sdb.QueryRow(`SELECT COALESCE(privacy,'') FROM users WHERE id=?`, uid).Scan(&raw)
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &privacy); err != nil {
			privacy = map[string]any{"age": 0, "gender": 0, "college": 1, "major": 1, "nativePlace": 1}
		}
	}
	return privacy
}

// pubIf 返回字段值，若该项公关时返回 *(若未公开返回空)
