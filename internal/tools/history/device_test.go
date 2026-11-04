package history

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func syncRequest(t *testing.T, handler http.HandlerFunc, token, batchID string, data map[string]any) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"batchId": batchID, "data": data})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/history/sync", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func samplePayload(now int64) map[string]any {
	return map[string]any{
		"format":  "unlimited-history-local-demo",
		"version": 1,
		"history": []any{map[string]any{
			"id": "browser:dev_test:1", "source": "browser", "title": "中文标题",
			"url": "https://example.com/a", "visited_at": now,
			"deviceId": "dev_test", "deviceName": "Chrome",
		}},
		"favorites": []any{map[string]any{
			"id": "bookmark:dev_test:9", "origin": "browser", "title": "书签",
			"url": "https://example.com/b", "folder": "toolbar / 学习",
			"updated_at": now, "deviceId": "dev_test", "deviceName": "Chrome",
		}},
	}
}

func TestExtensionDeviceIssuesSyncsAndRevokes(t *testing.T) {
	root := testStore(t)
	db := root.db
	now := time.Now().UnixMilli()

	token, id, err := IssueDeviceToken(db, 7, "家里的 Chrome")
	if err != nil || token == "" || id == 0 {
		t.Fatalf("issue token: %v", err)
	}
	list, err := ListDevices(db, 7)
	if err != nil || len(list) != 1 || list[0].Label != "家里的 Chrome" {
		t.Fatalf("list devices: %v %+v", err, list)
	}

	handler := SyncHandler(db, 1<<20)
	code, out := syncRequest(t, handler, token, "batch-1", samplePayload(now))
	if code != http.StatusOK || out["imported"] != float64(1) || out["favorites"] != float64(1) {
		t.Fatalf("first sync: code=%d out=%v", code, out)
	}

	// 同一批次同内容：幂等，不重复写入。
	code, out = syncRequest(t, handler, token, "batch-1", samplePayload(now))
	if code != http.StatusOK || out["duplicate"] != true {
		t.Fatalf("duplicate sync: code=%d out=%v", code, out)
	}

	// 数据落到该账号名下，且书签在 toolbar 分组。
	visits, err := root.ForUser(7).AllHistory()
	if err != nil || len(visits) != 1 || visits[0].Title != "中文标题" {
		t.Fatalf("history: %v %+v", err, visits)
	}
	favs, err := root.ForUser(7).AllFavorites()
	if err != nil || len(favs) != 1 || favs[0].Folder != "toolbar / 学习" {
		t.Fatalf("favorites: %v %+v", err, favs)
	}
	if other, _ := root.ForUser(8).AllHistory(); len(other) != 0 {
		t.Fatal("其他账号不应看到同步的数据")
	}

	devices, _ := ListDevices(db, 7)
	if len(devices) != 1 || devices[0].LastSync == 0 {
		t.Fatalf("last_sync 未更新: %+v", devices)
	}

	if err := RevokeDevice(db, 7, id); err != nil {
		t.Fatal(err)
	}
	code, _ = syncRequest(t, handler, token, "batch-2", samplePayload(now))
	if code != http.StatusUnauthorized {
		t.Fatalf("撤销后应拒绝同步，实际 %d", code)
	}
}

func TestExtensionSyncRejectsBadTokenAndPayload(t *testing.T) {
	root := testStore(t)
	handler := SyncHandler(root.db, 1<<20)

	if code, _ := syncRequest(t, handler, "", "b", samplePayload(1)); code != http.StatusUnauthorized {
		t.Fatalf("无令牌应为 401，实际 %d", code)
	}
	token, _, err := IssueDeviceToken(root.db, 7, "")
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := syncRequest(t, handler, "qw_deadbeef", "b", samplePayload(1)); code != http.StatusUnauthorized {
		t.Fatalf("伪造令牌应为 401，实际 %d", code)
	}
	code, _ := syncRequest(t, handler, token, "b", map[string]any{"format": "unknown"})
	if code != http.StatusBadRequest {
		t.Fatalf("不支持的数据格式应为 400，实际 %d", code)
	}
}

func TestExtensionZipHasManifestAndWorker(t *testing.T) {
	data, err := ExtensionZip("")
	if err != nil || len(data) < 500 {
		t.Fatalf("zip: %v len=%d", err, len(data))
	}
	raw := string(data)
	for _, want := range []string{"manifest.json", "background.js", "options.html"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("压缩包缺少 %s", want)
		}
	}
}

func TestDeviceLimitPerUser(t *testing.T) {
	root := testStore(t)
	for i := 0; i < maxDevicesPerUser; i++ {
		if _, _, err := IssueDeviceToken(root.db, 7, "设备"); err != nil {
			t.Fatalf("第 %d 台应允许: %v", i+1, err)
		}
	}
	if _, _, err := IssueDeviceToken(root.db, 7, "多余设备"); err == nil {
		t.Fatal("超过上限应拒绝")
	}
	// 其他账号不受影响
	if _, _, err := IssueDeviceToken(root.db, 8, "另一个账号"); err != nil {
		t.Fatalf("其他账号应可继续配对: %v", err)
	}
}
