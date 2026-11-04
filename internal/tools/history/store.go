// store.go - history persistence for the shared site database.
package history

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var ErrBatchConflict = errors.New("该批次已存在且内容不同，拒绝覆盖")

type Store struct {
	db     *sql.DB
	userID int64
}

func (s *Store) ForUser(userID int64) *Store { return &Store{db: s.db, userID: userID} }

func NewStore(db *sql.DB, legacyOwner ...int64) (*Store, error) {
	if db == nil {
		return nil, errors.New("history: 数据库未初始化")
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("history: 建表失败: %w", err)
	}
	for _, item := range [][2]string{{"history_batches", "user_id"}, {"history_records", "user_id"}, {"history_records", "path"}, {"history_bookmark_snapshots", "user_id"}, {"history_tombstones", "user_id"}} {
		if err := ensureColumn(db, item[0], item[1]); err != nil {
			return nil, fmt.Errorf("history: %s 迁移失败: %w", item[0], err)
		}
	}
	if len(legacyOwner) > 0 && legacyOwner[0] > 0 {
		if err := assignLegacyOwner(db, legacyOwner[0]); err != nil {
			return nil, err
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS history_records_user_day ON history_records(user_id,kind,day);
		CREATE INDEX IF NOT EXISTS history_records_user_path ON history_records(user_id,path);
		CREATE INDEX IF NOT EXISTS history_batches_user ON history_batches(user_id,writer,batch_id);
		CREATE INDEX IF NOT EXISTS history_snapshots_user ON history_bookmark_snapshots(user_id,id);
		CREATE INDEX IF NOT EXISTS history_tombstones_user ON history_tombstones(user_id,kind,id);
		CREATE TABLE IF NOT EXISTS history_site_dedupe(user_id INTEGER NOT NULL,path TEXT NOT NULL,last_seen INTEGER NOT NULL,PRIMARY KEY(user_id,path));`); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func assignLegacyOwner(db *sql.DB, ownerID int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range []string{"history_records", "history_batches", "history_bookmark_snapshots", "history_tombstones"} {
		if _, err := tx.Exec(`UPDATE `+table+` SET user_id=? WHERE user_id=0`, ownerID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ensureColumn(db *sql.DB, table, column string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var def any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &def, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	definition := ` INTEGER NOT NULL DEFAULT 0`
	if column == "path" {
		definition = ` TEXT NOT NULL DEFAULT ''`
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + definition)
	return err
}

type BatchResult struct {
	Duplicate  bool
	Imported   int
	Backfilled int
	Skipped    int
	Favorites  int
	Snapshot   bool
	Tombstones int
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS history_batches (
 user_id INTEGER NOT NULL DEFAULT 0, writer TEXT NOT NULL, batch_id TEXT NOT NULL, sha256 TEXT NOT NULL,
 source TEXT NOT NULL DEFAULT '', received_at TEXT NOT NULL, items INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(writer,batch_id));
CREATE TABLE IF NOT EXISTS history_records (
 user_id INTEGER NOT NULL DEFAULT 0, kind TEXT NOT NULL, id TEXT NOT NULL, path TEXT NOT NULL DEFAULT '',
 day TEXT NOT NULL DEFAULT '', ts INTEGER NOT NULL DEFAULT 0, host TEXT NOT NULL DEFAULT '', device TEXT NOT NULL DEFAULT '',
 title TEXT NOT NULL DEFAULT '', body TEXT NOT NULL, PRIMARY KEY(kind,id));
CREATE INDEX IF NOT EXISTS history_records_day ON history_records(kind,day);
CREATE INDEX IF NOT EXISTS history_records_host ON history_records(kind,host,day);
CREATE TABLE IF NOT EXISTS history_bookmark_snapshots (
 id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL DEFAULT 0, device TEXT NOT NULL DEFAULT '',
 imported_at TEXT NOT NULL, digest TEXT NOT NULL, payload TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS history_tombstones (
 user_id INTEGER NOT NULL DEFAULT 0, kind TEXT NOT NULL, id TEXT NOT NULL, deleted_at INTEGER NOT NULL DEFAULT 0,
 uid INTEGER NOT NULL DEFAULT 0, content_id INTEGER NOT NULL DEFAULT 0, business TEXT NOT NULL DEFAULT '',
 through INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(kind,id));
CREATE TABLE IF NOT EXISTS history_site_dedupe (
 user_id INTEGER NOT NULL, path TEXT NOT NULL, last_seen INTEGER NOT NULL, PRIMARY KEY(user_id,path));
`

func RecordSiteVisit(db *sql.DB, userID int64, path, title string, at time.Time) (bool, error) {
	return (&Store{db: db, userID: userID}).RecordSiteVisit(path, title, at)
}

func (s *Store) RecordSiteVisit(path, title string, at time.Time) (bool, error) {
	if s.db == nil || s.userID <= 0 || path == "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return false, nil
	}
	if at.IsZero() {
		at = time.Now()
	}
	ts := at.UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`INSERT INTO history_site_dedupe(user_id,path,last_seen) VALUES(?,?,?)
		ON CONFLICT(user_id,path) DO UPDATE SET last_seen=excluded.last_seen
		WHERE excluded.last_seen-history_site_dedupe.last_seen>=60000`, s.userID, path, ts)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	h := History{ID: fmt.Sprintf("%d:%s", ts, path), Source: "site", Title: title, URL: path, VisitedAt: ts}
	body, err := json.Marshal(h)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(`INSERT OR IGNORE INTO history_records(user_id,kind,id,path,day,ts,host,device,title,body) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		s.userID, "site", scopedID(s.userID, h.ID), path, at.Local().Format("2006-01-02"), ts, "", "website", title, string(body))
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func scopedID(userID int64, id string) string {
	if userID <= 0 {
		return id
	}
	return fmt.Sprintf("u%d:%s", userID, id)
}

func (s *Store) Ingest(writer, batchID, sha, source string, res *ImportResult) (*BatchResult, error) {
	if s.userID > 0 {
		writer, batchID = scopedID(s.userID, writer), scopedID(s.userID, batchID)
	}
	if res == nil {
		return nil, errors.New("history: 空导入")
	}
	if writer == "" || batchID == "" {
		return nil, errors.New("history: 批次缺少 writer 或 id")
	}
	var existSha string
	err := s.db.QueryRow(`SELECT sha256 FROM history_batches WHERE user_id=? AND writer=? AND batch_id=?`, s.userID, writer, batchID).Scan(&existSha)
	switch {
	case err == nil:
		if existSha == sha {
			return &BatchResult{Duplicate: true}, nil
		}
		return nil, ErrBatchConflict
	case !errors.Is(err, sql.ErrNoRows):
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	out := &BatchResult{Tombstones: len(res.Tombstones)}
	for _, t := range res.Tombstones {
		id := t.ID
		if s.userID > 0 {
			id = scopedID(s.userID, id)
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO history_tombstones(user_id,kind,id,deleted_at,uid,content_id,business,through) VALUES(?,?,?,?,?,?,?,?)`, s.userID, t.Kind, id, t.DeletedAt, t.UID, t.ContentID, t.Business, t.Through); err != nil {
			return nil, err
		}
	}
	for _, h := range res.History {
		inserted, backfilled, err := upsertHistory(tx, s.userID, h)
		if err != nil {
			return nil, err
		}
		switch {
		case inserted:
			out.Imported++
		case backfilled:
			out.Backfilled++
		default:
			out.Skipped++
		}
	}
	if len(res.Favorites) > 0 {
		snapshot, err := saveFavoritesSnapshot(tx, s.userID, res.Favorites)
		if err != nil {
			return nil, err
		}
		out.Snapshot, out.Favorites = snapshot, len(res.Favorites)
	}
	if _, err := tx.Exec(`INSERT INTO history_batches(user_id,writer,batch_id,sha256,source,received_at,items) VALUES(?,?,?,?,?,?,?)`, s.userID, writer, batchID, sha, source, time.Now().UTC().Format(time.RFC3339), res.Total()); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func upsertHistory(tx *sql.Tx, userID int64, h History) (inserted, backfilled bool, err error) {
	body, err := json.Marshal(h)
	if err != nil {
		return false, false, err
	}
	day, ts := dayOf(h.VisitedAt), h.VisitedAt
	dbID := scopedID(userID, h.ID)
	res, err := tx.Exec(`INSERT OR IGNORE INTO history_records(user_id,kind,id,path,day,ts,host,device,title,body) VALUES(?,?,?,?,?,?,?,?,?,?)`, userID, kindOf(h.Source), dbID, h.URL, day, ts, hostOf(h.URL), h.DeviceID, h.Title, string(body))
	if err != nil {
		return false, false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, false, nil
	}
	var oldBody string
	if err := tx.QueryRow(`SELECT body FROM history_records WHERE user_id=? AND kind=? AND id=?`, userID, kindOf(h.Source), dbID).Scan(&oldBody); err != nil {
		return false, false, err
	}
	var old History
	if err := json.Unmarshal([]byte(oldBody), &old); err != nil {
		return false, false, err
	}
	if !mergeDisplayMeta(&old, h) {
		return false, false, nil
	}
	merged, err := json.Marshal(old)
	if err != nil {
		return false, false, err
	}
	if _, err := tx.Exec(`UPDATE history_records SET body=?,title=? WHERE user_id=? AND kind=? AND id=?`, string(merged), old.Title, userID, kindOf(h.Source), dbID); err != nil {
		return false, false, err
	}
	return false, true, nil
}

func mergeDisplayMeta(old *History, fresh History) bool {
	changed := false
	if old.Cover == "" && fresh.Cover != "" {
		old.Cover, changed = fresh.Cover, true
	} else if strings.HasPrefix(old.Cover, "http://") && strings.HasPrefix(fresh.Cover, "https://") {
		// 旧记录可能是 http 封面（CSP 会拦掉），新数据是 https 时顺手升级。
		old.Cover, changed = fresh.Cover, true
	}
	if old.AuthorName == "" && fresh.AuthorName != "" {
		old.AuthorName, changed = fresh.AuthorName, true
	}
	if old.AuthorMid == 0 && fresh.AuthorMid != 0 {
		old.AuthorMid, changed = fresh.AuthorMid, true
	}
	if old.Duration == 0 && fresh.Duration != 0 {
		old.Duration, changed = fresh.Duration, true
	}
	if old.Progress == 0 && fresh.Progress != 0 {
		old.Progress, changed = fresh.Progress, true
	}
	if !old.IsFav && fresh.IsFav {
		old.IsFav, changed = true, true
	}
	if old.TagName == "" && fresh.TagName != "" {
		old.TagName, changed = fresh.TagName, true
	}
	return changed
}

func saveFavoritesSnapshot(tx *sql.Tx, userID int64, favorites []Favorite) (bool, error) {
	digest, payload, err := favoritesDigest(favorites)
	if err != nil {
		return false, err
	}
	var last string
	err = tx.QueryRow(`SELECT digest FROM history_bookmark_snapshots WHERE user_id=? ORDER BY id DESC LIMIT 1`, userID).Scan(&last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if last == digest {
		return false, nil
	}
	devices := map[string]bool{}
	for _, f := range favorites {
		if f.DeviceID != "" {
			devices[f.DeviceID] = true
		}
	}
	device := ""
	if len(devices) == 1 {
		for d := range devices {
			device = d
		}
	}
	_, err = tx.Exec(`INSERT INTO history_bookmark_snapshots(user_id,device,imported_at,digest,payload) VALUES(?,?,?,?,?)`, userID, device, time.Now().UTC().Format(time.RFC3339), digest, payload)
	return err == nil, err
}

func favoritesDigest(favorites []Favorite) (digest, payload string, err error) {
	sorted := append([]Favorite(nil), favorites...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	raw, err := json.Marshal(sorted)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), string(raw), nil
}

func (s *Store) LatestFavorites() ([]Favorite, error) {
	var payload string
	err := s.db.QueryRow(`SELECT payload FROM history_bookmark_snapshots WHERE user_id=? ORDER BY id DESC LIMIT 1`, s.userID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Favorite
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Store) SnapshotCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM history_bookmark_snapshots WHERE user_id=?`, s.userID).Scan(&n)
	return n, err
}
func (s *Store) Tombstones() ([]Tombstone, error) {
	rows, err := s.db.Query(`SELECT kind,id,deleted_at,uid,content_id,business,through FROM history_tombstones WHERE user_id=?`, s.userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tombstone
	for rows.Next() {
		var t Tombstone
		if err := rows.Scan(&t.Kind, &t.ID, &t.DeletedAt, &t.UID, &t.ContentID, &t.Business, &t.Through); err != nil {
			return nil, err
		}
		if s.userID > 0 {
			t.ID = strings.TrimPrefix(t.ID, fmt.Sprintf("u%d:", s.userID))
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s *Store) Counts() (records, batches, tombstones int, err error) {
	err = s.db.QueryRow(`SELECT(SELECT COUNT(*) FROM history_records WHERE user_id=?),(SELECT COUNT(*) FROM history_batches WHERE user_id=?),(SELECT COUNT(*) FROM history_tombstones WHERE user_id=?)`, s.userID, s.userID, s.userID).Scan(&records, &batches, &tombstones)
	return
}
func kindOf(source string) string {
	switch source {
	case "bilibili":
		return "bilibili"
	case "site":
		return "site"
	default:
		return "browser"
	}
}
func dayOf(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02")
}
func hostOf(raw string) string {
	i := strings.Index(raw, "://")
	if i < 0 {
		return ""
	}
	rest := raw[i+3:]
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest = rest[:j]
	}
	if k := strings.LastIndex(rest, ":"); k >= 0 && !strings.Contains(rest[k:], "]") {
		rest = rest[:k]
	}
	return strings.ToLower(rest)
}
