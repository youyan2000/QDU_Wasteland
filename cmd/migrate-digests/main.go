// 命令 migrate-digests 把旧的 tool-data/*.json 快照导入 SQLite（表 digest_issues / hot_issues）。
//
//	go run ./cmd/migrate-digests -db .cache\s1demo\qdu-auth.db -dir tool-data
//
// 幂等：同名快照会覆盖，可重复执行。
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"wasteland/internal/digestdb"
)

func main() {
	dbPath := flag.String("db", "", "站点数据库路径（必填）")
	dir := flag.String("dir", "tool-data", "旧的快照目录")
	flag.Parse()
	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "用法: go run ./cmd/migrate-digests -db <数据库> [-dir tool-data]")
		os.Exit(2)
	}
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	total := 0
	for _, sub := range []struct{ dir, prefix string }{
		{"digest", "digest"},
		{"hot", "hot"},
	} {
		full := filepath.Join(*dir, sub.dir)
		entries, err := os.ReadDir(full)
		if err != nil {
			fmt.Printf("跳过 %s（%v）\n", full, err)
			continue
		}
		kv := digestdb.Store{DB: db, Prefix: sub.prefix}
		n := 0
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(full, e.Name()))
			if err != nil {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".json") // YYYY-MM-DD 或 latest
			if err := kv.Save(name, raw); err != nil {
				fmt.Fprintf(os.Stderr, "写入 %s/%s 失败: %v\n", sub.prefix, name, err)
				continue
			}
			n++
		}
		fmt.Printf("✅ %s_issues 导入 %d 份快照\n", sub.prefix, n)
		total += n
	}
	fmt.Printf("合计 %d 份。旧 JSON 文件可保留作备份，也可删除。\n", total)
}
