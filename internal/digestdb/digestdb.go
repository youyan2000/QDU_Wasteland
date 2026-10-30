// 包 digestdb 把日报快照存进站点唯一的 SQLite 库（表名 <prefix>_issues）。
//
// 两个日报（开源 / 热点）以前各写一堆 JSON 文件到 tool-data/，与"单 SQLite"目标不符；
// 这里提供一个极小的键值层：name = 日期（YYYY-MM-DD）或 "latest"，payload = 原始 JSON。
package digestdb

import (
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// Store 是一个表的前缀封装。
type Store struct {
	DB     *sql.DB
	Prefix string // 例如 "digest" → 表 digest_issues
}

// table 返回带前缀的表名。
func (s Store) table() string { return s.Prefix + "_issues" }

// Ensure 建表（幂等）。
func (s Store) Ensure() error {
	if s.DB == nil {
		return nil
	}
	_, err := s.DB.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		name       TEXT PRIMARY KEY,
		payload    TEXT NOT NULL,
		saved_at   TEXT NOT NULL
	)`, s.table()))
	return err
}

// Save 写入一份快照（同 name 覆盖）。
func (s Store) Save(name string, payload []byte) error {
	if s.DB == nil {
		return nil
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	_, err := s.DB.Exec(fmt.Sprintf(
		`INSERT INTO %s(name,payload,saved_at) VALUES(?,?,?)
		 ON CONFLICT(name) DO UPDATE SET payload=excluded.payload, saved_at=excluded.saved_at`,
		s.table()), name, string(payload), time.Now().UTC().Format(time.RFC3339))
	return err
}

// Load 读取一份快照。
func (s Store) Load(name string) ([]byte, error) {
	if s.DB == nil {
		return nil, sql.ErrNoRows
	}
	if err := s.Ensure(); err != nil {
		return nil, err
	}
	var payload string
	if err := s.DB.QueryRow(fmt.Sprintf(`SELECT payload FROM %s WHERE name=?`, s.table()), name).Scan(&payload); err != nil {
		return nil, err
	}
	return []byte(payload), nil
}

// Names 返回保存过的日期（不含 latest），新→旧，最多 limit 个。
func (s Store) Names(limit int) ([]string, error) {
	if s.DB == nil {
		return nil, nil
	}
	if err := s.Ensure(); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(fmt.Sprintf(`SELECT name FROM %s WHERE name <> 'latest'`, s.table()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, rows.Err()
}

// Prune 只保留最近 keep 天（外加 latest）。
func (s Store) Prune(keep int) error {
	if s.DB == nil || keep <= 0 {
		return nil
	}
	names, err := s.Names(0)
	if err != nil {
		return err
	}
	for i := keep; i < len(names); i++ {
		if _, err := s.DB.Exec(fmt.Sprintf(`DELETE FROM %s WHERE name=?`, s.table()), names[i]); err != nil {
			return err
		}
	}
	return nil
}
