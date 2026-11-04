package history

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 本站足迹接口必须按登录用户隔离，并返回页面标题与路径。
func TestSiteVisitsAPIIsScopedPerUser(t *testing.T) {
	root := testStore(t)
	if _, err := root.ForUser(7).RecordSiteVisit("/article.html?id=3", "文章三", time.Now()); err != nil {
		t.Fatal(err)
	}

	server := NewServer(root, func(r *http.Request) int64 {
		if r.Header.Get("X-Test-User") == "7" {
			return 7
		}
		return 8
	})

	cases := []struct {
		user  string
		want  int
		title string
	}{
		{"7", 1, "文章三"},
		{"8", 0, ""},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/site-visits", nil)
		req.Header.Set("X-Test-User", c.user)
		rec := httptest.NewRecorder()
		server.handleSiteVisits(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("user %s: status %d body %s", c.user, rec.Code, rec.Body.String())
		}
		var payload struct {
			Items []History `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Items) != c.want {
			t.Fatalf("user %s: got %d items, want %d", c.user, len(payload.Items), c.want)
		}
		if c.want == 1 && payload.Items[0].Title != c.title {
			t.Fatalf("title = %q, want %q", payload.Items[0].Title, c.title)
		}
	}
}
