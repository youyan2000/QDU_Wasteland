package history

import (
	"encoding/json"
	"testing"
)

// B站 接口偶尔不返回 uid，同一段观看会拿到两个不同 ID；
// 入库必须按「内容 + 观看秒」去重，否则补历史时同一条记录会写两遍。
func TestBilibiliSameItemDifferentUIDIsDeduped(t *testing.T) {
	s := testStore(t)
	at := int64(1790000000000)
	a := History{
		ID: "bili:0:archive:12345:1790000000", Source: "bilibili",
		Title: "某个视频", URL: "https://www.bilibili.com/video/BV1xx",
		VisitedAt: at, Business: "archive", ContentID: 12345,
	}
	b := a
	b.ID = "bili:42:archive:12345:1790000000"
	b.UID = 42

	res := &ImportResult{History: []History{a, b}}
	if _, err := s.Ingest("writer", "batch-1", "sha", "ext", res); err != nil {
		t.Fatalf("入库失败: %v", err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM history_records WHERE kind='bilibili'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("同一内容仅 uid 不同，应该只存 1 行，实际 %d 行", n)
	}
}

// 已经被写重（历史遗留）的数据，启动迁移要把重复行合并掉。
func TestDedupeBilibiliRowsMergesLegacyDuplicates(t *testing.T) {
	s := testStore(t)
	// 清掉迁移标记，模拟老库第一次升级。
	if _, err := s.db.Exec(`DELETE FROM history_meta WHERE key='bili_dedupe_v1'`); err != nil {
		t.Fatal(err)
	}
	at := int64(1790000000000)
	body := func(id string, uid int64) string {
		h := History{
			ID: id, Source: "bilibili", Title: "视频", URL: "https://www.bilibili.com/video/BV1",
			VisitedAt: at, Business: "archive", ContentID: 777, UID: uid,
		}
		b, _ := json.Marshal(h)
		return string(b)
	}
	rows := []struct {
		id  string
		uid int64
	}{
		{"1:bili:0:archive:777:1790000000", 0},
		{"1:bili:42:archive:777:1790000000", 42},
	}
	for _, r := range rows {
		if _, err := s.db.Exec(`INSERT INTO history_records(user_id,kind,id,path,day,ts,host,device,title,body)
			VALUES(1,'bilibili',?,?,?,?,?,?,?,?)`,
			r.id, "https://www.bilibili.com/video/BV1", "2026-10-31", at, "www.bilibili.com", "", "视频", body(r.id, r.uid)); err != nil {
			t.Fatal(err)
		}
	}
	if err := dedupeBilibiliRows(s.db); err != nil {
		t.Fatalf("去重迁移失败: %v", err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM history_records WHERE kind='bilibili'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("重复的 B站 记录应合并为 1 行，实际 %d 行", n)
	}
	var id string
	if err := s.db.QueryRow(`SELECT id FROM history_records WHERE kind='bilibili'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	want := "u1:bili:archive:777:1790000000"
	if id != want {
		t.Fatalf("合并后 ID 应为规范格式 %q，实际 %q", want, id)
	}
}
