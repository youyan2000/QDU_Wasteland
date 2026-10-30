// 命令 devseed 往本地演示库里塞几条"可复现"的数据，供安全/隐私类改动做端到端验证：
//
//	go run ./cmd/devseed -db ..\.cache\s1demo\qdu-auth.db
//
// 会创建（可能重复插入，重复执行前请用 -reset 或换库）：
//   - 一个资料用户：学院/专业公开，年龄/性别/籍贯默认不公开，帖子列表显式关闭
//   - 一篇匿名文章（用于验证详情接口不下发真实 userId）
//   - 两份资料：一份「正常」、一份「待复核」（用于验证读取侧是否拦截）
//
// 它只写演示库，不碰线上数据。
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func must(err error, what string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %s: %v\n", what, err)
		os.Exit(1)
	}
}

func main() {
	dbPath := flag.String("db", "", "演示库路径（必填），如 ..\\.cache\\s1demo\\qdu-auth.db")
	uploadDir := flag.String("uploads", "uploads", "uploads 目录（相对当前工作目录）")
	flag.Parse()
	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "用法: go run ./cmd/devseed -db <演示库路径>")
		os.Exit(2)
	}

	db, err := sql.Open("sqlite", *dbPath)
	must(err, "打开数据库")
	defer db.Close()

	now := time.Now().Format("2006-01-02 15:04:05")

	// 1) 资料用户：内容显隐里 posts=0（用户主动隐藏），年龄/性别/籍贯不设（默认不公开）
	// 幂等：邮箱唯一，存在则更新资料，便于反复执行。
	priv := `{"age":0,"gender":0,"college":1,"major":1,"nativePlace":0,"posts":0,"articles":1}`
	_, err = db.Exec(`INSERT INTO users(email, username, password, nickname, college, major, age, gender, native_place, privacy, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(email) DO UPDATE SET nickname=excluded.nickname, college=excluded.college,
			major=excluded.major, age=excluded.age, gender=excluded.gender,
			native_place=excluded.native_place, privacy=excluded.privacy`,
		"devseed@qdu.edu.cn", "devseed", "x", "演示同学", "计算机科学技术学院", "软件工程", 20, "女", "山东青岛", priv, now)
	must(err, "写入演示用户")
	var uid int64
	must(db.QueryRow(`SELECT id FROM users WHERE email=?`, "devseed@qdu.edu.cn").Scan(&uid), "查询演示用户")

	// 2) 匿名文章（按标题判重）
	var aid int64
	if err := db.QueryRow(`SELECT id FROM articles WHERE title=? AND user_id=?`, "匿名发的文章", uid).Scan(&aid); err != nil {
		res, err := db.Exec(`INSERT INTO articles(title, category, content, is_anonymous, status, user_id, created_at, reject_reason, cover, views)
			VALUES(?,?,?,?,?,?,?,?,?,?)`,
			"匿名发的文章", "生活", "这条是演示数据：详情接口不应返回真实作者 ID。", 1, "正常", uid, now, "", "", 0)
		must(err, "插入匿名文章")
		aid, _ = res.LastInsertId()
	}

	// 2b) 该用户还有一条正常帖子 —— 用来证明「显式关闭 posts」确实能把列表清空
	var pid int64
	if err := db.QueryRow(`SELECT id FROM posts WHERE title=? AND user_id=?`, "演示帖子（应当被隐私开关隐藏）", uid).Scan(&pid); err != nil {
		res, err := db.Exec(`INSERT INTO posts(forum, user_id, title, content, is_anonymous, likes, status, created_at)
			VALUES(?,?,?,?,?,?,?,?)`,
			"help", uid, "演示帖子（应当被隐私开关隐藏）", "这条帖子存在，但主人关闭了帖子展示。", 0, 0, "正常", now)
		must(err, "插入演示帖子")
		pid, _ = res.LastInsertId()
	}

	// 3) 资料：一份正常、一份待复核
	must(os.MkdirAll(*uploadDir, 0o755), "创建 uploads 目录")
	okName, pendingName := "devseed-ok.txt", "devseed-pending.txt"
	must(os.WriteFile(filepath.Join(*uploadDir, okName), []byte("normal file\n"), 0o644), "写入正常文件")
	must(os.WriteFile(filepath.Join(*uploadDir, pendingName), []byte("pending review file\n"), 0o644), "写入待复核文件")

	insertFile := func(stored, title, status string) int64 {
		var id int64
		if err := db.QueryRow(`SELECT id FROM files WHERE stored_name=?`, stored).Scan(&id); err == nil {
			_, _ = db.Exec(`UPDATE files SET status=? WHERE id=?`, status, id)
			return id
		}
		res, err := db.Exec(`INSERT INTO files(course_code,title,category,teacher,semester,description,file_name,stored_name,size,uploader_id,is_anonymous,created_at,status)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			"4081120203018", title, "学习笔记", "王老师", "2025春", "devseed 演示数据", stored, stored, 16, uid, 0, now, status)
		must(err, "插入资料 "+status)
		newID, _ := res.LastInsertId()
		return newID
	}
	okID := insertFile(okName, "正常资料", "正常")
	pendingID := insertFile(pendingName, "待复核资料", "待复核")

	fmt.Println("✅ 演示数据已写入")
	fmt.Printf("   用户 ID      : %d\n", uid)
	fmt.Printf("   匿名文章 ID  : %d\n", aid)
	fmt.Printf("   正常资料 ID  : %d\n", okID)
	fmt.Printf("   待复核资料 ID: %d\n", pendingID)
	fmt.Println("   建议验证：")
	fmt.Printf("     curl /api/user/%d          → age=0 gender=\"\" nativePlace=\"\"，posts 为空\n", uid)
	fmt.Printf("     curl /api/articles/%d      → userId=0\n", aid)
	fmt.Printf("     curl /api/files/%d/download → 200\n", okID)
	fmt.Printf("     curl /api/files/%d/download → 404\n", pendingID)
}
