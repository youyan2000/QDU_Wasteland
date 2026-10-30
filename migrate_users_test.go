// migrate_test.go — users 表迁移的回归测试（P0-3：重启不得把未验证用户洗成已验证）。
package main

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openMigrationTestDB 打开一个独立的内存库，避免用例之间互相污染。
func openMigrationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:migtest_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestMigrateUsersTableKeepsUnverifiedAcrossRestarts 复现审计 P0-3 的场景：
// 稳态库（已含 email_verified 列）中，一个未验证用户经过一次「重启」（再跑一次迁移）后
// 必须仍然是未验证。
func TestMigrateUsersTableKeepsUnverifiedAcrossRestarts(t *testing.T) {
	db := openMigrationTestDB(t)

	// 模拟线上稳态表结构：列都存在。
	stmts := []string{
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL DEFAULT '',
			is_admin INTEGER NOT NULL DEFAULT 0,
			email_verified INTEGER NOT NULL DEFAULT 0
		)`,
		`INSERT INTO users(email, password_hash, is_admin, email_verified) VALUES('new@qdu.edu.cn','x',0,0)`,
		`INSERT INTO users(email, password_hash, is_admin, email_verified) VALUES('old@qdu.edu.cn','x',0,1)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("准备数据失败: %v\nSQL: %s", err, s)
		}
	}

	// 连跑三次迁移 = 模拟三次服务重启。
	for i := 0; i < 3; i++ {
		if err := migrateUsersTable(db); err != nil {
			t.Fatalf("第 %d 次迁移失败: %v", i+1, err)
		}
	}

	var unverified int
	if err := db.QueryRow(`SELECT email_verified FROM users WHERE email='new@qdu.edu.cn'`).Scan(&unverified); err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if unverified != 0 {
		t.Fatalf("未验证用户在重启后被洗成已验证（email_verified=%d），P0-3 回归", unverified)
	}

	var verified int
	if err := db.QueryRow(`SELECT email_verified FROM users WHERE email='old@qdu.edu.cn'`).Scan(&verified); err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if verified != 1 {
		t.Fatalf("原本已验证的账号不应被改动（email_verified=%d）", verified)
	}
}

// TestMigrateUsersTableBackfillsOnlyOnColumnCreation 首次加列时，存量老账号应被回填为已验证。
func TestMigrateUsersTableBackfillsOnlyOnColumnCreation(t *testing.T) {
	db := openMigrationTestDB(t)

	stmts := []string{
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL DEFAULT '',
			is_admin INTEGER NOT NULL DEFAULT 0
		)`,
		`INSERT INTO users(email, password_hash, is_admin) VALUES('legacy@qdu.edu.cn','x',0)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("准备数据失败: %v", err)
		}
	}

	if err := migrateUsersTable(db); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	var got int
	if err := db.QueryRow(`SELECT email_verified FROM users WHERE email='legacy@qdu.edu.cn'`).Scan(&got); err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if got != 1 {
		t.Fatalf("首次加列时应把存量账号视为已验证，got %d", got)
	}

	// 之后再插入的新用户不会被回填。
	if _, err := db.Exec(`INSERT INTO users(email, password_hash, is_admin, email_verified) VALUES('fresh@qdu.edu.cn','x',0,0)`); err != nil {
		t.Fatalf("插入新用户失败: %v", err)
	}
	if err := migrateUsersTable(db); err != nil {
		t.Fatalf("二次迁移失败: %v", err)
	}
	var fresh int
	if err := db.QueryRow(`SELECT email_verified FROM users WHERE email='fresh@qdu.edu.cn'`).Scan(&fresh); err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if fresh != 0 {
		t.Fatalf("加列之后注册的新用户不应被回填，got %d", fresh)
	}
}
