package history

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := fmt.Sprintf("file:historytest_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err := NewStore(db)
	if err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return s
}

func biliRecord(id string, visited int64) History {
	return History{
		ID: "bili:" + id, Source: "bilibili",
		Title: "视频 " + id, URL: "https://www.bilibili.com/video/" + id,
		VisitedAt: visited, Business: "archive",
	}
}

func TestIngestIsIdempotentPerBatch(t *testing.T) {
	s := testStore(t)
	res := &ImportResult{History: []History{biliRecord("1", 1000)}}

	first, err := s.Ingest("pc-a", "batch-1", "sha-1", "upload", res)
	if err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}
	if first.Imported != 1 {
		t.Fatalf("首次应导入 1 条，实际 %d", first.Imported)
	}

	again, err := s.Ingest("pc-a", "batch-1", "sha-1", "upload", res)
	if err != nil {
		t.Fatalf("重复入库失败: %v", err)
	}
	if !again.Duplicate {
		t.Fatal("同名同内容批次应被识别为重复，不重复写入")
	}

	records, batches, _, _ := s.Counts()
	if records != 1 || batches != 1 {
		t.Fatalf("重复导入不应增加行数: records=%d batches=%d", records, batches)
	}
}

func TestIngestRejectsBatchConflict(t *testing.T) {
	s := testStore(t)
	res := &ImportResult{History: []History{biliRecord("1", 1000)}}
	if _, err := s.Ingest("pc-a", "batch-1", "sha-1", "upload", res); err != nil {
		t.Fatal(err)
	}
	_, err := s.Ingest("pc-a", "batch-1", "sha-2", "upload", res)
	if err == nil {
		t.Fatal("同批次不同内容应拒绝覆盖")
	}
}

func TestHistoryDedupesAcrossBatches(t *testing.T) {
	s := testStore(t)
	h := biliRecord("1", 1000)
	if _, err := s.Ingest("pc-a", "b1", "s1", "upload", &ImportResult{History: []History{h}}); err != nil {
		t.Fatal(err)
	}
	// 另一批带了同一条记录（比如另一台设备也采到了）。
	res, err := s.Ingest("pc-b", "b2", "s2", "upload", &ImportResult{History: []History{h}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 0 || res.Skipped != 1 {
		t.Fatalf("同一条记录不应重复入库: %+v", res)
	}
	n, _, _, _ := s.Counts()
	if n != 1 {
		t.Fatalf("记录数应为 1，实际 %d", n)
	}
}

func TestHistoryBackfillsMissingMeta(t *testing.T) {
	s := testStore(t)
	bare := biliRecord("1", 1000) // 没有封面/UP主/时长

	if _, err := s.Ingest("pc-a", "b1", "s1", "upload", &ImportResult{History: []History{bare}}); err != nil {
		t.Fatal(err)
	}

	rich := bare
	rich.Cover = "https://i0.hdslb.com/a.jpg"
	rich.AuthorName = "UP主"
	rich.Duration = 300
	rich.Progress = -1
	rich.IsFav = true
	res, err := s.Ingest("pc-a", "b2", "s2", "upload", &ImportResult{History: []History{rich}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Backfilled != 1 {
		t.Fatalf("应补全 1 条，实际 %+v", res)
	}

	var body string
	if err := s.db.QueryRow(`SELECT body FROM history_records WHERE id=?`, bare.ID).Scan(&body); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"hdslb.com", "UP主", "300"} {
		if !contains(body, want) {
			t.Errorf("补全后缺少 %q: %s", want, body)
		}
	}
	// 已看完的 -1 不能被当成缺失丢掉。
	if !contains(body, `"progress":-1`) {
		t.Errorf("progress=-1 应保留: %s", body)
	}
}

func TestHistoryNeverRewritesIdentity(t *testing.T) {
	s := testStore(t)
	orig := biliRecord("1", 1000)
	orig.Title = "原始标题"
	if _, err := s.Ingest("pc-a", "b1", "s1", "upload", &ImportResult{History: []History{orig}}); err != nil {
		t.Fatal(err)
	}
	changed := orig
	changed.Title = "改名后的标题"
	changed.URL = "https://evil.example/x"
	if _, err := s.Ingest("pc-a", "b2", "s2", "upload", &ImportResult{History: []History{changed}}); err != nil {
		t.Fatal(err)
	}
	var body, title string
	if err := s.db.QueryRow(`SELECT body,title FROM history_records WHERE id=?`, orig.ID).Scan(&body, &title); err != nil {
		t.Fatal(err)
	}
	if title != "原始标题" {
		t.Errorf("标题不应被后来批次改写，实际 %q", title)
	}
	if contains(body, "evil.example") {
		t.Error("URL 不应被后来批次改写")
	}
}

func TestFavoritesSnapshotOnlyWhenChanged(t *testing.T) {
	s := testStore(t)
	fav := []Favorite{{
		ID: "bookmark:pc-a:3", Origin: "browser", Title: "Go", URL: "https://go.dev/",
		DeviceID: "pc-a", Folder: "工具栏 / 技术", UpdatedAt: 1000,
	}}

	first, err := s.Ingest("pc-a", "b1", "s1", "upload", &ImportResult{Favorites: fav})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Snapshot {
		t.Fatal("首次导入应留下快照")
	}

	second, err := s.Ingest("pc-a", "b2", "s2", "upload", &ImportResult{Favorites: fav})
	if err != nil {
		t.Fatal(err)
	}
	if second.Snapshot {
		t.Fatal("列表未变化时不应再留快照")
	}

	fav2 := append([]Favorite{}, fav...)
	fav2 = append(fav2, Favorite{ID: "bookmark:pc-a:4", Origin: "browser", Title: "Rust",
		URL: "https://rust-lang.org/", DeviceID: "pc-a", UpdatedAt: 2000})
	third, err := s.Ingest("pc-a", "b3", "s3", "upload", &ImportResult{Favorites: fav2})
	if err != nil {
		t.Fatal(err)
	}
	if !third.Snapshot {
		t.Fatal("列表变化后应留新快照")
	}

	n, err := s.SnapshotCount()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("应有 2 份快照，实际 %d", n)
	}
	latest, err := s.LatestFavorites()
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 2 {
		t.Fatalf("最新快照应有 2 条，实际 %d", len(latest))
	}
}

func TestTombstonesStored(t *testing.T) {
	s := testStore(t)
	res := &ImportResult{Tombstones: []Tombstone{
		{Kind: "history", ID: "bili:1:archive:9:100", DeletedAt: 2000},
		{Kind: "bilibili-content", ID: "1:archive:9", UID: 1, ContentID: 9, Business: "archive", Through: 100},
	}}
	if _, err := s.Ingest("pc-a", "b1", "s1", "upload", res); err != nil {
		t.Fatal(err)
	}
	got, err := s.Tombstones()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("应存 2 条墓碑，实际 %d", len(got))
	}
}

func TestUserStoresAreIsolated(t *testing.T) {
	root := testStore(t)
	userA, userB := root.ForUser(11), root.ForUser(22)
	res := &ImportResult{History: []History{biliRecord("same-id", 1700000000000)}}
	if _, err := userA.Ingest("pc", "batch", "sha", "upload", res); err != nil {
		t.Fatal(err)
	}
	if _, err := userB.Ingest("pc", "batch", "sha", "upload", res); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		store *Store
		want  int
	}{{userA, 1}, {userB, 1}, {root, 0}} {
		got, err := tc.store.AllHistory()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != tc.want {
			t.Fatalf("user %d: got %d records, want %d", tc.store.userID, len(got), tc.want)
		}
	}
	if _, err := userA.Ingest("pc", "batch", "sha", "upload", res); err != nil {
		t.Fatal(err)
	}
	if n, _, _, err := userA.Counts(); err != nil || n != 1 {
		t.Fatalf("same-user idempotence failed: count=%d err=%v", n, err)
	}
}

func TestRecordSiteVisitDeduplicatesPerUserAndPath(t *testing.T) {
	root := testStore(t)
	at := time.Date(2026, 11, 3, 10, 0, 0, 0, time.Local)
	first, err := root.ForUser(7).RecordSiteVisit("/article.html?id=3", "文章三", at)
	if err != nil || !first {
		t.Fatalf("first visit: recorded=%v err=%v", first, err)
	}
	duplicate, err := root.ForUser(7).RecordSiteVisit("/article.html?id=3", "文章三", at.Add(30*time.Second))
	if err != nil || duplicate {
		t.Fatalf("duplicate should be skipped: recorded=%v err=%v", duplicate, err)
	}
	otherUser, err := root.ForUser(8).RecordSiteVisit("/article.html?id=3", "文章三", at.Add(30*time.Second))
	if err != nil || !otherUser {
		t.Fatalf("other user should have own visit: recorded=%v err=%v", otherUser, err)
	}
	later, err := root.ForUser(7).RecordSiteVisit("/article.html?id=3", "文章三", at.Add(61*time.Second))
	if err != nil || !later {
		t.Fatalf("visit after dedupe window should be stored: recorded=%v err=%v", later, err)
	}
	visits, err := root.ForUser(7).AllSiteVisits()
	if err != nil || len(visits) != 2 {
		t.Fatalf("user 7 visits=%d err=%v, want 2", len(visits), err)
	}
}

func TestHostAndDayDerivation(t *testing.T) {
	if h := hostOf("https://www.bilibili.com/video/BV1xx?p=1"); h != "www.bilibili.com" {
		t.Errorf("host 解析不符: %q", h)
	}
	if h := hostOf("https://example.com:8443/a"); h != "example.com" {
		t.Errorf("带端口应去掉端口: %q", h)
	}
	if d := dayOf(1700000000000); d != "2023-11-14" {
		t.Errorf("日期换算不符: %q", d)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
