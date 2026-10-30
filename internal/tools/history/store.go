// store.go — history 工具的持久化层（单库，表名统一 history_ 前缀）。
//
// 三条语义必须守住，它们来自上游 unlimited_history 的设计：
//  1. 历史「只累加不覆盖」：同一条记录重复导入不会翻倍，但缺失的展示元数据
//     （封面 / UP主 / 时长 / 进度 / 类型）会在再次导入时补全；
//  2. 收藏是「整表快照」：每次有效导入留下当时完整列表的一份快照，内容未变则不留新版；
//  3. 删除用墓碑：随归档同步，防止旧备份把已删记录复活。
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

// ErrBatchConflict 表示同一个 (writer, batchId) 已经以不同内容入库过。
var ErrBatchConflict = errors.New("该批次已存在且内容不同，拒绝覆盖")

// Store 是 history 的持久化入口。
type Store struct{ db *sql.DB }

// NewStore 建表并返回 Store。
func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("history: 数据库未初始化")
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("history: 建表失败: %w", err)
	}
	return &Store{db: db}, nil
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS history_batches (
  writer      TEXT NOT NULL,
  batch_id    TEXT NOT NULL,
  sha256      TEXT NOT NULL,
  source      TEXT NOT NULL DEFAULT '',
  received_at TEXT NOT NULL,
  items       INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (writer, batch_id)
);
CREATE TABLE IF NOT EXISTS history_records (
  kind   TEXT NOT NULL,
  id     TEXT NOT NULL,
  day    TEXT NOT NULL DEFAULT '',
  ts     INTEGER NOT NULL DEFAULT 0,
  host   TEXT NOT NULL DEFAULT '',
  device TEXT NOT NULL DEFAULT '',
  title  TEXT NOT NULL DEFAULT '',
  body   TEXT NOT NULL,
  PRIMARY KEY (kind, id)
);
CREATE INDEX IF NOT EXISTS history_records_day  ON history_records(kind, day);
CREATE INDEX IF NOT EXISTS history_records_host ON history_records(kind, host, day);
CREATE TABLE IF NOT EXISTS history_bookmark_snapshots (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  device      TEXT NOT NULL DEFAULT '',
  imported_at TEXT NOT NULL,
  digest      TEXT NOT NULL,
  payload     TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS history_tombstones (
  kind       TEXT NOT NULL,
  id         TEXT NOT NULL,
  deleted_at INTEGER NOT NULL DEFAULT 0,
  uid        INTEGER NOT NULL DEFAULT 0,
  content_id INTEGER NOT NULL DEFAULT 0,
  business   TEXT NOT NULL DEFAULT '',
  through    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (kind, id)
);
`

// BatchResult 描述一次入库的结果。
type BatchResult struct {
	Duplicate  bool // 整批已存在（内容一致），什么都没写
	Imported   int  // 新增的历史记录
	Backfilled int  // 已存在但补全了展示元数据的记录
	Skipped    int  // 已存在且无需补全的记录
	Favorites  int  // 本次收藏条目数
	Snapshot   bool // 是否新留了一份书签快照
	Tombstones int
}

// Ingest 把一次规范化后的导入结果写入库中（幂等）。
//
// writer/batchID 决定幂等键；sha 是原始批次的哈希，用于识别"同名不同内容"的冲突。
func (s *Store) Ingest(writer, batchID, sha, source string, res *ImportResult) (*BatchResult, error) {
	if res == nil {
		return nil, errors.New("history: 空导入")
	}
	if writer == "" || batchID == "" {
		return nil, errors.New("history: 批次缺少 writer 或 id")
	}

	// 幂等台账：同批次同内容直接返回；同批次不同内容拒绝覆盖。
	var existSha string
	err := s.db.QueryRow(`SELECT sha256 FROM history_batches WHERE writer=? AND batch_id=?`,
		writer, batchID).Scan(&existSha)
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
		if _, err := tx.Exec(`INSERT INTO history_tombstones(kind,id,deleted_at,uid,content_id,business,through)
			VALUES(?,?,?,?,?,?,?)
			ON CONFLICT(kind,id) DO UPDATE SET deleted_at=excluded.deleted_at,
				uid=excluded.uid, content_id=excluded.content_id,
				business=excluded.business, through=excluded.through`,
			t.Kind, t.ID, t.DeletedAt, t.UID, t.ContentID, t.Business, t.Through); err != nil {
			return nil, err
		}
	}

	for _, h := range res.History {
		inserted, backfilled, err := upsertHistory(tx, h)
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
		snapshot, err := saveFavoritesSnapshot(tx, res.Favorites)
		if err != nil {
			return nil, err
		}
		out.Snapshot = snapshot
		out.Favorites = len(res.Favorites)
	}

	if _, err := tx.Exec(`INSERT INTO history_batches(writer,batch_id,sha256,source,received_at,items)
		VALUES(?,?,?,?,?,?)`,
		writer, batchID, sha, source, time.Now().UTC().Format(time.RFC3339), res.Total()); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// upsertHistory 写入一条历史；已存在时只补全展示元数据。
func upsertHistory(tx *sql.Tx, h History) (inserted, backfilled bool, err error) {
	body, err := json.Marshal(h)
	if err != nil {
		return false, false, err
	}
	day, ts := dayOf(h.VisitedAt), h.VisitedAt
	host := hostOf(h.URL)
	device := h.DeviceID

	res, err := tx.Exec(`INSERT OR IGNORE INTO history_records(kind,id,day,ts,host,device,title,body)
		VALUES(?,?,?,?,?,?,?,?)`,
		kindOf(h.Source), h.ID, day, ts, host, device, h.Title, string(body))
	if err != nil {
		return false, false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, false, nil
	}

	// 已存在：只在"缺失的展示元数据"上做补全，身份与标题时间永不改写。
	var oldBody string
	if err := tx.QueryRow(`SELECT body FROM history_records WHERE kind=? AND id=?`,
		kindOf(h.Source), h.ID).Scan(&oldBody); err != nil {
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
	if _, err := tx.Exec(`UPDATE history_records SET body=?, title=? WHERE kind=? AND id=?`,
		string(merged), old.Title, kindOf(h.Source), h.ID); err != nil {
		return false, false, err
	}
	return false, true, nil
}

// mergeDisplayMeta 把新记录里"有值而旧记录为空"的展示字段补进去，返回是否发生变化。
func mergeDisplayMeta(old *History, fresh History) bool {
	changed := false
	if old.Cover == "" && fresh.Cover != "" {
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
	// progress 的 0 与 -1（已看完）都是有效值，只在旧值为 0 时补。
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

// saveFavoritesSnapshot 内容有变化时才留新快照；返回是否新留了一份。
func saveFavoritesSnapshot(tx *sql.Tx, favorites []Favorite) (bool, error) {
	digest, payload, err := favoritesDigest(favorites)
	if err != nil {
		return false, err
	}
	var last string
	err = tx.QueryRow(`SELECT digest FROM history_bookmark_snapshots ORDER BY id DESC LIMIT 1`).Scan(&last)
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
	if _, err := tx.Exec(`INSERT INTO history_bookmark_snapshots(device,imported_at,digest,payload)
		VALUES(?,?,?,?)`, device, time.Now().UTC().Format(time.RFC3339), digest, payload); err != nil {
		return false, err
	}
	return true, nil
}

// favoritesDigest 对收藏做稳定排序后取哈希，作为"列表是否变化"的指纹。
func favoritesDigest(favorites []Favorite) (digest, payload string, err error) {
	sorted := make([]Favorite, len(favorites))
	copy(sorted, favorites)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	raw, err := json.Marshal(sorted)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), string(raw), nil
}

// LatestFavorites 返回最近一份收藏快照。
func (s *Store) LatestFavorites() ([]Favorite, error) {
	var payload string
	err := s.db.QueryRow(`SELECT payload FROM history_bookmark_snapshots ORDER BY id DESC LIMIT 1`).Scan(&payload)
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

// SnapshotCount 返回收藏快照份数。
func (s *Store) SnapshotCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM history_bookmark_snapshots`).Scan(&n)
	return n, err
}

// Tombstones 读出全部墓碑。
func (s *Store) Tombstones() ([]Tombstone, error) {
	rows, err := s.db.Query(`SELECT kind,id,deleted_at,uid,content_id,business,through FROM history_tombstones`)
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
		out = append(out, t)
	}
	return out, rows.Err()
}

// Counts 返回各表行数（后台巡检 / 测试用）。
func (s *Store) Counts() (records, batches, tombstones int, err error) {
	err = s.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM history_records),
		(SELECT COUNT(*) FROM history_batches),
		(SELECT COUNT(*) FROM history_tombstones)`).Scan(&records, &batches, &tombstones)
	return
}

// ---------- 小工具 ----------

func kindOf(source string) string {
	if source == "bilibili" {
		return "bilibili"
	}
	return "browser"
}

func dayOf(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02")
}

// hostOf 取 URL 的主机名，用于按站点筛选。
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
