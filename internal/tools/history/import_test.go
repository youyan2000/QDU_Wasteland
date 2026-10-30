package history

import (
	"strings"
	"testing"
)

func TestImportBilibiliSchema3(t *testing.T) {
	data := `{
	  "schemaVersion": 3,
	  "events": [
	    {"uid": 42, "business": "archive", "id": 1001, "view_at": 1700000000,
	     "bvid": "BV1xx411c7mD", "title": " 一条视频 ", "cover": "https://i0.hdslb.com/a.jpg",
	     "author_name": "UP主", "duration": 300, "progress": -1, "is_fav": true}
	  ],
	  "tombstones": [
	    {"scope": "event", "uid": 42, "event_id": "42:archive:1002:1700000100", "deleted_at": 1700000200}
	  ]
	}`
	res, err := NormalizeImport([]byte(data))
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(res.History) != 1 {
		t.Fatalf("应导入 1 条历史，实际 %d", len(res.History))
	}
	h := res.History[0]
	if h.ID != "bili:42:archive:1001:1700000000" {
		t.Errorf("历史 id 不符: %s", h.ID)
	}
	if h.URL != "https://www.bilibili.com/video/BV1xx411c7mD" {
		t.Errorf("应由 bvid 生成链接，实际 %s", h.URL)
	}
	if h.Title != "一条视频" {
		t.Errorf("标题应去首尾空白，实际 %q", h.Title)
	}
	if h.VisitedAt != 1700000000000 {
		t.Errorf("访问时间应换算成毫秒，实际 %d", h.VisitedAt)
	}
	// progress = -1 表示"已看完"，是有效值，不能被当成缺失。
	if h.Progress != -1 {
		t.Errorf("progress=-1 应保留，实际 %d", h.Progress)
	}
	if !h.IsFav {
		t.Error("is_fav 应保留")
	}

	if len(res.Tombstones) != 1 || res.Tombstones[0].ID != "bili:42:archive:1002:1700000100" {
		t.Fatalf("墓碑 id 不符: %+v", res.Tombstones)
	}
}

func TestImportBilibiliSchema2Warns(t *testing.T) {
	data := `{"schemaVersion": 2, "events": [{"business":"archive","id":7,"view_at":100,"bvid":"BV1abc"}]}`
	res, err := NormalizeImport([]byte(data))
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("schema2 应给出「账号归属不可信」的警告")
	}
	if res.History[0].ID != "bili:0:archive:7:100" {
		t.Errorf("schema2 应归 UID 0，实际 %s", res.History[0].ID)
	}
}

func TestImportBrowserHistory(t *testing.T) {
	data := `{
	  "format": "browser-history-archive", "version": 1,
	  "devices": [{"id": "pc-a", "name": "家里的电脑"}],
	  "visits": [
	    {"id": "pc-a:13", "deviceId": "pc-a", "nativeVisitId": "13",
	     "title": "示例", "url": "https://example.com/", "visitTime": 1700000000000}
	  ],
	  "tombstones": [{"id": "pc-a:9", "deletedAt": 1700000001000}]
	}`
	res, err := NormalizeImport([]byte(data))
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(res.History) != 1 {
		t.Fatalf("应导入 1 条，实际 %d", len(res.History))
	}
	h := res.History[0]
	if h.ID != "browser:pc-a:13" || h.DeviceName != "家里的电脑" {
		t.Errorf("浏览器历史字段不符: %+v", h)
	}
	if len(res.Tombstones) != 1 || res.Tombstones[0].ID != "browser:pc-a:9" {
		t.Errorf("墓碑不符: %+v", res.Tombstones)
	}
}

func TestImportBrowserHistoryRejectsMismatchedID(t *testing.T) {
	data := `{
	  "format": "browser-history-archive", "version": 1,
	  "visits": [{"id": "pc-a:13", "deviceId": "pc-b", "nativeVisitId": "13",
	              "url": "https://example.com/", "visitTime": 1}]
	}`
	if _, err := NormalizeImport([]byte(data)); err == nil {
		t.Fatal("deviceId 与 id 不一致时应拒绝整批")
	}
}

func TestImportBookmarksBuildsFolderPath(t *testing.T) {
	data := `{
	  "type": "browser-bookmarks", "version": 1,
	  "records": [
	    {"id": "pc-a:1", "deviceId": "pc-a", "nativeId": "1", "kind": "folder", "title": "工具栏", "parentId": ""},
	    {"id": "pc-a:2", "deviceId": "pc-a", "nativeId": "2", "kind": "folder", "title": "技术", "parentId": "1"},
	    {"id": "pc-a:3", "deviceId": "pc-a", "nativeId": "3", "kind": "bookmark",
	     "title": "Go 官网", "url": "https://go.dev/", "parentId": "2", "updatedAt": 1700000000000}
	  ]
	}`
	res, err := NormalizeImport([]byte(data))
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(res.Favorites) != 1 {
		t.Fatalf("只应导入书签（folder/separator 不入库），实际 %d", len(res.Favorites))
	}
	f := res.Favorites[0]
	if f.ID != "bookmark:pc-a:3" {
		t.Errorf("收藏 id 不符: %s", f.ID)
	}
	if f.Folder != "工具栏 / 技术" {
		t.Errorf("目录路径不符: %q", f.Folder)
	}
}

func TestImportPersonalArchive(t *testing.T) {
	data := `{
	  "format": "unlimited-history-personal", "version": 1,
	  "bilibili": {"schemaVersion": 3, "events": [{"uid":1,"business":"live","id":9,"view_at":5}]},
	  "browserHistory": {"format":"browser-history-archive","version":1,"visits":[
	     {"id":"d:1","deviceId":"d","nativeVisitId":"1","url":"https://a.com/","visitTime":2}]},
	  "browserBookmarks": {"type":"browser-bookmarks","version":1,"records":[
	     {"id":"d:7","deviceId":"d","nativeId":"7","kind":"bookmark","title":"B","url":"https://b.com/","updatedAt":3}]}
	}`
	res, err := NormalizeImport([]byte(data))
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(res.History) != 2 || len(res.Favorites) != 1 {
		t.Fatalf("个人档案应导入 2 条历史 + 1 条收藏，实际 %d/%d", len(res.History), len(res.Favorites))
	}
	// live 业务没有 uri 时应落到直播间地址。
	if !strings.Contains(res.History[0].URL, "live.bilibili.com/9") {
		t.Errorf("直播地址不符: %s", res.History[0].URL)
	}
}

func TestImportRejectsUnknownFormat(t *testing.T) {
	if _, err := NormalizeImport([]byte(`{"foo":1}`)); err == nil {
		t.Fatal("未知格式应报错")
	}
}

func TestImportRejectsInvalidBusiness(t *testing.T) {
	data := `{"schemaVersion":3,"events":[{"uid":1,"business":"hack","id":1,"view_at":1}]}`
	if _, err := NormalizeImport([]byte(data)); err == nil {
		t.Fatal("非法业务类型应拒绝整批")
	}
}

func TestImportRejectsInvalidBVID(t *testing.T) {
	data := `{"schemaVersion":3,"events":[{"uid":1,"business":"archive","id":1,"view_at":1,"bvid":"XX1"}]}`
	if _, err := NormalizeImport([]byte(data)); err == nil {
		t.Fatal("非法 BVID 应拒绝")
	}
}
