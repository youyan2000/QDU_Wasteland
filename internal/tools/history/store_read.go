// store_read.go — 读取侧：把记录读出来，并按墓碑过滤。
//
// 个人工具的数据量在十万级以内，直接全量读进内存再过滤，与上游 demo 的做法一致；
// 这样查询逻辑简单，也不会因为多写 SQL 而和上游语义漂移。
package history

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// AllHistory 返回全部外部历史（已剔除墓碑命中的记录）。
func (s *Store) AllHistory() ([]History, error) {
	return s.historyByKinds("bilibili", "browser")
}

func (s *Store) AllSiteVisits() ([]History, error) { return s.historyByKinds("site") }

func (s *Store) AllHistoryCombined() ([]History, error) {
	return s.historyByKinds("bilibili", "browser", "site")
}

func (s *Store) historyByKinds(kinds ...string) ([]History, error) {
	placeholders := strings.TrimRight(strings.Repeat("?,", len(kinds)), ",")
	args := make([]any, 0, len(kinds)+1)
	args = append(args, s.userID)
	for _, kind := range kinds {
		args = append(args, kind)
	}
	rows, err := s.db.Query(`SELECT body FROM history_records WHERE user_id=? AND kind IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []History
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		var h History
		if err := json.Unmarshal([]byte(body), &h); err != nil {
			continue // 单条损坏不应让整页挂掉
		}
		if s.userID > 0 {
			h.ID = strings.TrimPrefix(h.ID, fmt.Sprintf("u%d:", s.userID))
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	marks, err := s.tombstoneIndex()
	if err != nil {
		return nil, err
	}
	if len(marks.history) == 0 && len(marks.biliContent) == 0 && len(marks.biliLegacy) == 0 {
		return out, nil
	}
	kept := out[:0]
	for _, h := range out {
		if !blocked(h, marks) {
			kept = append(kept, h)
		}
	}
	return kept, nil
}

// AllFavorites 返回最新一份收藏快照的内容。
func (s *Store) AllFavorites() ([]Favorite, error) {
	list, err := s.LatestFavorites()
	if err != nil {
		return nil, err
	}
	return list, nil
}

// SnapshotMeta 是收藏快照列表里的一项（不含条目本体）。
type SnapshotMeta struct {
	ID         int64  `json:"id"`
	Device     string `json:"device"`
	ImportedAt string `json:"importedAt"`
}

// FavoriteSnapshots 返回全部收藏快照的元信息（新→旧）。
func (s *Store) FavoriteSnapshots() ([]SnapshotMeta, error) {
	rows, err := s.db.Query(`SELECT id,device,imported_at FROM history_bookmark_snapshots WHERE user_id=? ORDER BY id DESC`, s.userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SnapshotMeta
	for rows.Next() {
		var m SnapshotMeta
		if err := rows.Scan(&m.ID, &m.Device, &m.ImportedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// FavoritesAt 返回指定快照的条目。
func (s *Store) FavoritesAt(snapshotID int64) ([]Favorite, error) {
	var payload string
	err := s.db.QueryRow(`SELECT payload FROM history_bookmark_snapshots WHERE user_id=? AND id=?`, s.userID, snapshotID).Scan(&payload)
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

// tombstoneIndex 按 kind 归并墓碑，便于 O(1) 判定。
type tombstoneIndex struct {
	history     map[string]bool  // 命中的历史 id
	biliContent map[string]int64 // "uid:business:contentId" -> through(秒)
	biliLegacy  map[string]int64 // "uid:contentId" -> through(秒)
}

func (s *Store) tombstoneIndex() (*tombstoneIndex, error) {
	list, err := s.Tombstones()
	if err != nil {
		return nil, err
	}
	idx := &tombstoneIndex{
		history:     map[string]bool{},
		biliContent: map[string]int64{},
		biliLegacy:  map[string]int64{},
	}
	for _, t := range list {
		switch t.Kind {
		case "history":
			idx.history[t.ID] = true
		case "bilibili-content":
			idx.biliContent[t.ID] = t.Through
		case "bilibili-legacy":
			idx.biliLegacy[t.ID] = t.Through
		}
	}
	return idx, nil
}

// blocked 复刻上游 isBlocked 的语义：
//   - history 墓碑按 id 精确命中；
//   - bilibili-content / legacy 墓碑按「内容」屏蔽，且只屏蔽观看时间不晚于 through 的记录。
func blocked(h History, idx *tombstoneIndex) bool {
	if idx.history[h.ID] {
		return true
	}
	if h.Source != "bilibili" {
		return false
	}
	contentKey := ""
	if h.UID != 0 || h.Business != "" || h.ContentID != 0 {
		contentKey = itoa(h.UID) + ":" + h.Business + ":" + itoa(h.ContentID)
	}
	legacyKey := itoa(h.UID) + ":" + itoa(h.ContentID)

	if through, ok := idx.biliContent[contentKey]; ok && h.VisitedAt <= through*1000 {
		return true
	}
	if through, ok := idx.biliLegacy[legacyKey]; ok && h.VisitedAt <= through*1000 {
		return true
	}
	return false
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// SortHistoryDesc 按访问时间倒序（同时间按 id 稳定排序），与上游一致。
func SortHistoryDesc(list []History) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].VisitedAt != list[j].VisitedAt {
			return list[i].VisitedAt > list[j].VisitedAt
		}
		return list[i].ID < list[j].ID
	})
}

// SortFavoritesDesc 按更新时间倒序。
func SortFavoritesDesc(list []Favorite) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].UpdatedAt != list[j].UpdatedAt {
			return list[i].UpdatedAt > list[j].UpdatedAt
		}
		return list[i].ID < list[j].ID
	})
}
