// 命令 seedhistory 往 demo 库里塞一批 history 工具的数据，供端到端验证：
//
//	go run ./cmd/seedhistory -db ..\.cache\s1demo\qdu-auth.db
//
// 它直接调用 history 包的导入 + 入库路径（与线上完全相同），不手工拼 SQL。
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"

	"wasteland/internal/tools/history"
)

func main() {
	dbPath := flag.String("db", "", "演示库路径（必填）")
	flag.Parse()
	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "用法: go run ./cmd/seedhistory -db <演示库路径>")
		os.Exit(2)
	}
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	store, err := history.NewStore(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化 history 存储失败: %v\n", err)
		os.Exit(1)
	}

	now := time.Now()
	at := func(daysAgo int) int64 { return now.AddDate(0, 0, -daysAgo).UnixMilli() }

	// 一份「浏览器历史 + B站历史 + 收藏」的完整个人档案。
	doc := fmt.Sprintf(`{
	  "format": "unlimited-history-personal", "version": 1,
	  "bilibili": {
	    "schemaVersion": 3,
	    "events": [
	      {"uid": 42, "business": "archive", "id": 1001, "view_at": %d,
	       "bvid": "BV1xx411c7mD", "title": "演示视频：Go 并发模型",
	       "cover": "https://i0.hdslb.com/bfs/archive/demo.jpg",
	       "author_name": "演示UP主", "duration": 720, "progress": -1, "is_fav": true},
	      {"uid": 42, "business": "live", "id": 2002, "view_at": %d,
	       "title": "演示直播间", "author_name": "主播"}
	    ]
	  },
	  "browserHistory": {
	    "format": "browser-history-archive", "version": 1,
	    "devices": [{"id": "pc-demo", "name": "演示电脑"}],
	    "visits": [
	      {"id": "pc-demo:11", "deviceId": "pc-demo", "nativeVisitId": "11",
	       "title": "Go 官方文档", "url": "https://go.dev/doc/", "visitTime": %d},
	      {"id": "pc-demo:12", "deviceId": "pc-demo", "nativeVisitId": "12",
	       "title": "SQLite 文档", "url": "https://sqlite.org/docs.html", "visitTime": %d},
	      {"id": "pc-demo:13", "deviceId": "pc-demo", "nativeVisitId": "13",
	       "title": "MDN", "url": "https://developer.mozilla.org/", "visitTime": %d}
	    ]
	  },
	  "browserBookmarks": {
	    "type": "browser-bookmarks", "version": 1,
	    "records": [
	      {"id": "pc-demo:1", "deviceId": "pc-demo", "nativeId": "1", "kind": "folder", "title": "toolbar", "parentId": ""},
	      {"id": "pc-demo:2", "deviceId": "pc-demo", "nativeId": "2", "kind": "folder", "title": "技术", "parentId": "1"},
	      {"id": "pc-demo:3", "deviceId": "pc-demo", "nativeId": "3", "kind": "bookmark",
	       "title": "Go 官网", "url": "https://go.dev/", "parentId": "2", "updatedAt": %d}
	    ]
	  }
	}`, now.Add(-2*time.Hour).Unix()/1000, now.Add(-30*time.Hour).Unix()/1000,
		at(0), at(1), at(3), at(1))

	res, err := history.NormalizeImport([]byte(doc))
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析失败: %v\n", err)
		os.Exit(1)
	}
	batch := fmt.Sprintf("seed-%d", now.UnixNano())
	out, err := store.Ingest("pc-demo", batch, batch, "manual", res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "入库失败: %v\n", err)
		os.Exit(1)
	}

	records, batches, tombstones, _ := store.Counts()
	fmt.Printf("✅ history 演示数据已写入\n")
	fmt.Printf("   本次：新增历史 %d 条 / 跳过 %d / 收藏 %d 条 / 快照 %v\n",
		out.Imported, out.Skipped, out.Favorites, out.Snapshot)
	fmt.Printf("   累计：记录 %d / 批次 %d / 墓碑 %d\n", records, batches, tombstones)
}
