package placeholders

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
)

func bookmarkTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:bookmarks_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE tool_bookmarks(id INTEGER PRIMARY KEY AUTOINCREMENT,user_id INTEGER NOT NULL,title TEXT NOT NULL,url TEXT NOT NULL,description TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,UNIQUE(user_id,url))`); err != nil {
		t.Fatal(err)
	}
	return db
}
func TestBookmarksAreScopedToUser(t *testing.T) {
	db := bookmarkTestDB(t)
	for _, uid := range []int64{11, 22} {
		reqBody, _ := json.Marshal(bookmarkInput{Title: "Example", URL: "https://example.com"})
		r := httptest.NewRequest(http.MethodPost, "/api/bookmarks", bytes.NewReader(reqBody))
		w := httptest.NewRecorder()
		handleBookmarks(db, func(*http.Request) int64 { return uid }, w, r)
		if w.Code != 201 {
			t.Fatalf("post: %d %s", w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	w := httptest.NewRecorder()
	handleBookmarks(db, func(*http.Request) int64 { return 11 }, w, r)
	var got struct {
		Items []bookmark `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(got.Items) != 1 {
		t.Fatalf("user 11 sees %d records", len(got.Items))
	}
}
func TestBookmarkRejectsNonHTTPURL(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "file:///tmp/a", "//example.com", "data:text/plain,hi"} {
		b := bookmarkInput{Title: "x", URL: raw}
		if err := validateBookmark(&b); err == nil {
			t.Errorf("accepted URL %q", raw)
		}
	}
}
func TestBookmarkUpdateAndDeleteCannotCrossUsers(t *testing.T) {
	db := bookmarkTestDB(t)
	res, err := db.Exec(`INSERT INTO tool_bookmarks(user_id,title,url,description,created_at,updated_at) VALUES(1,'A','https://a.example','','now','now')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		var body []byte
		if method == http.MethodPut {
			body, _ = json.Marshal(bookmarkInput{Title: "owned", URL: "https://owned.example"})
		}
		r := httptest.NewRequest(method, "/api/bookmarks/"+jsonNumber(id), bytes.NewReader(body))
		w := httptest.NewRecorder()
		handleBookmarkItem(db, func(*http.Request) int64 { return 2 }, w, r)
		if w.Code != 404 {
			t.Errorf("%s cross-user status %d", method, w.Code)
		}
	}
}
func jsonNumber(v int64) string { b, _ := json.Marshal(v); return string(b) }
