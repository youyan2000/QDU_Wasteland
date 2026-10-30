// api.go — history 工具的 HTTP 接口（全部挂在 /tool/history/api/ 下，仅站长可用）。
//
// 接口形状照抄上游 demo：前端是原样复用的，改字段名等于改前端的命。
package history

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	previewTTL   = 10 * time.Minute
	previewLimit = 3
	maxBodyBytes = 20 * 1024 * 1024
)

// previewEntry 是「预导入」暂存：先给用户看条数与警告，确认后才真正写库。
type previewEntry struct {
	data    *ImportResult
	expires time.Time
}

var (
	previewMu sync.Mutex
	previews  = map[string]previewEntry{}
)

// ---------- 响应助手 ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func param(r *http.Request, key, def string) string {
	if v := strings.TrimSpace(r.URL.Query().Get(key)); v != "" {
		return v
	}
	return def
}

func paramInt(r *http.Request, key string, def int64) int64 {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return def
	}
	return n
}

// dayKey 把毫秒时间戳换算成本地日期（yyyy-MM-dd）。
func dayKey(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).In(time.Local).Format("2006-01-02")
}

// dayParam 校验 yyyy-MM-dd。
func dayParam(raw string) (string, bool) {
	if raw == "" {
		return "", true
	}
	t, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return "", false
	}
	return t.Format("2006-01-02"), true
}

func shiftDay(key string, delta int) string {
	t, err := time.ParseInLocation("2006-01-02", key, time.Local)
	if err != nil {
		return key
	}
	return t.AddDate(0, 0, delta).Format("2006-01-02")
}

// ---------- /api/me ----------

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"loggedIn": true,
		"demo":     false,
		"access":   "owner",
		"warning":  "",
	})
}

// ---------- /api/stats ----------

type dayCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type hostCount struct {
	Host  string `json:"host"`
	Count int    `json:"count"`
}

type deviceCount struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	source := param(r, "source", "all")
	if source != "all" && source != "browser" && source != "bilibili" {
		writeErr(w, 400, "未知的历史来源")
		return
	}
	days := paramInt(r, "days", 120)
	if days < 7 || days > 1825 {
		writeErr(w, 400, "days 参数无效")
		return
	}
	from, ok1 := dayParam(r.URL.Query().Get("from"))
	to, ok2 := dayParam(r.URL.Query().Get("to"))
	if !ok1 || !ok2 {
		writeErr(w, 400, "日期参数无效")
		return
	}
	if from != "" && to != "" && from > to {
		writeErr(w, 400, "日期范围颠倒")
		return
	}
	domain := strings.ToLower(param(r, "domain", ""))

	all, err := s.store.AllHistory()
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}

	counts := map[string]int{}
	byHour := make([]int, 24)
	byWeekday := make([]int, 7)
	devices := map[string]int{}
	domains := map[string]int{}
	inRange := func(at int64) bool {
		if from == "" && to == "" {
			return true
		}
		k := dayKey(at)
		return (from == "" || k >= from) && (to == "" || k <= to)
	}

	var total, domainTotal int
	var firstAt, lastAt int64
	for _, h := range all {
		if source != "all" && h.Source != source {
			continue
		}
		if h.VisitedAt <= 0 {
			continue
		}
		host := hostOf(h.URL)
		if host != "" {
			domains[host]++
		}
		domainTotal++
		// 站点清单基于未按域名过滤的集合，否则选中后无法切回其它站点。
		if domain != "" && host != domain && !strings.HasSuffix(host, "."+domain) {
			continue
		}
		// 曲线与日历是总览，不跟随日期窗口。
		counts[dayKey(h.VisitedAt)]++
		if !inRange(h.VisitedAt) {
			continue
		}
		total++
		t := time.UnixMilli(h.VisitedAt).In(time.Local)
		byHour[t.Hour()]++
		byWeekday[int(t.Weekday())]++
		if h.DeviceID != "" {
			devices[h.DeviceID]++
		}
		if firstAt == 0 || h.VisitedAt < firstAt {
			firstAt = h.VisitedAt
		}
		if h.VisitedAt > lastAt {
			lastAt = h.VisitedAt
		}
	}

	today := time.Now().In(time.Local).Format("2006-01-02")
	series := make([]dayCount, 0, days)
	for offset := int(days) - 1; offset >= 0; offset-- {
		d := shiftDay(today, -offset)
		series = append(series, dayCount{Date: d, Count: counts[d]})
	}

	devList := make([]deviceCount, 0, len(devices))
	for id, n := range devices {
		devList = append(devList, deviceCount{ID: id, Count: n})
	}
	sort.Slice(devList, func(i, j int) bool { return devList[i].Count > devList[j].Count })

	hostList := make([]hostCount, 0, len(domains))
	for h, n := range domains {
		hostList = append(hostList, hostCount{Host: h, Count: n})
	}
	sort.Slice(hostList, func(i, j int) bool {
		if hostList[i].Count != hostList[j].Count {
			return hostList[i].Count > hostList[j].Count
		}
		return hostList[i].Host < hostList[j].Host
	})
	if len(hostList) > 400 {
		hostList = hostList[:400]
	}

	writeJSON(w, 200, map[string]any{
		"source": source, "domain": domain, "from": from, "to": to,
		"total": total, "domainTotal": domainTotal,
		"firstAt": firstAt, "lastAt": lastAt,
		"days": series, "byHour": byHour, "byWeekday": byWeekday,
		"devices": devList, "domains": hostList,
	})
}

// ---------- /api/history ----------

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(param(r, "q", ""))
	source := param(r, "source", "")
	if source != "" && source != "browser" && source != "bilibili" {
		writeErr(w, 400, "未知的历史来源")
		return
	}
	cursor, limit := paramInt(r, "cursor", 0), paramInt(r, "limit", 50)
	if cursor < 0 || limit < 1 || limit > 100 {
		writeErr(w, 400, "分页参数无效")
		return
	}
	from, ok1 := dayParam(r.URL.Query().Get("from"))
	to, ok2 := dayParam(r.URL.Query().Get("to"))
	if !ok1 || !ok2 {
		writeErr(w, 400, "日期参数无效")
		return
	}
	if from != "" && to != "" && from > to {
		writeErr(w, 400, "日期范围颠倒")
		return
	}
	domain := strings.ToLower(param(r, "domain", ""))

	all, err := s.store.AllHistory()
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	matched := make([]History, 0, len(all))
	for _, h := range all {
		if q != "" && !strings.Contains(strings.ToLower(
			strings.Join([]string{h.Title, h.URL, h.AuthorName, h.Business, h.TagName, h.DeviceName}, " ")), q) {
			continue
		}
		if source != "" && h.Source != source {
			continue
		}
		if from != "" || to != "" {
			k := dayKey(h.VisitedAt)
			if (from != "" && k < from) || (to != "" && k > to) {
				continue
			}
		}
		if domain != "" {
			host := hostOf(h.URL)
			if host != domain && !(strings.HasSuffix(host, "."+domain) && len(host) > len(domain)+1) {
				continue
			}
		}
		matched = append(matched, h)
	}
	SortHistoryDesc(matched)

	page, next := paginate(len(matched), int(cursor), int(limit))
	writeJSON(w, 200, map[string]any{
		"items": matched[page[0]:page[1]], "total": len(matched), "nextCursor": next,
		"from": from, "to": to, "domain": domain, "source": orAll(source),
	})
}

// ---------- /api/favorite ----------

func (s *Server) handleFavorites(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(param(r, "q", ""))
	cursor, limit := paramInt(r, "cursor", 0), paramInt(r, "limit", 50)
	if cursor < 0 || limit < 1 || limit > 100 {
		writeErr(w, 400, "分页参数无效")
		return
	}
	origin := param(r, "origin", "all")
	status := param(r, "status", "all")
	if (origin != "all" && origin != "browser" && origin != "manual") ||
		(status != "all" && status != "active" && status != "deleted") {
		writeErr(w, 400, "收藏筛选条件无效")
		return
	}
	folder := param(r, "folder", "")
	sub := r.URL.Query().Get("sub") == "1"
	root := param(r, "root", "toolbar")
	includeDeleted := r.URL.Query().Get("includeDeleted") == "1"
	snapshotID := paramInt(r, "snapshot", 0)

	list, err := s.store.AllFavorites()
	if snapshotID > 0 {
		list, err = s.store.FavoritesAt(snapshotID)
		if err != nil {
			writeErr(w, 500, "读取失败")
			return
		}
		if list == nil {
			writeErr(w, 404, "收藏版本不存在")
			return
		}
	} else if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}

	inRoot := func(f Favorite) bool {
		if root == "" {
			return true
		}
		p := f.Folder
		return p == root || strings.HasPrefix(p, root+" / ")
	}
	inFolder := func(f Favorite) bool {
		if folder == "" {
			return true
		}
		if sub {
			return f.Folder == folder || strings.HasPrefix(f.Folder, folder+" / ")
		}
		return f.Folder == folder
	}

	matched := make([]Favorite, 0, len(list))
	for _, f := range list {
		if q != "" {
			hay := strings.ToLower(strings.Join([]string{f.Title, f.URL, f.Note, f.Folder, f.DeviceName}, " ") +
				" " + strings.Join(f.Tags, " "))
			if !strings.Contains(hay, q) {
				continue
			}
		}
		if !inRoot(f) || !inFolder(f) {
			continue
		}
		if !includeDeleted && f.DeletedAt != 0 {
			continue
		}
		if origin != "all" && f.Origin != origin {
			continue
		}
		switch status {
		case "deleted":
			if f.DeletedAt == 0 {
				continue
			}
		case "active":
			if f.DeletedAt != 0 {
				continue
			}
		}
		matched = append(matched, f)
	}
	SortFavoritesDesc(matched)

	page, next := paginate(len(matched), int(cursor), int(limit))
	writeJSON(w, 200, map[string]any{
		"items": matched[page[0]:page[1]], "total": len(matched), "nextCursor": next,
		"snapshot": snapshotID, "root": root,
	})
}

// ---------- /api/favorite/folders ----------

type folderCount struct {
	Path   string `json:"path"`
	Count  int    `json:"count"`
	Latest int64  `json:"latest"`
}

func (s *Server) handleFavoriteFolders(w http.ResponseWriter, r *http.Request) {
	origin := param(r, "origin", "browser")
	if origin != "all" && origin != "browser" && origin != "manual" {
		writeErr(w, 400, "收藏筛选条件无效")
		return
	}
	root := param(r, "root", "toolbar")
	list, err := s.store.AllFavorites()
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	folders := map[string]*folderCount{}
	loose := 0
	for _, f := range list {
		if f.DeletedAt != 0 {
			continue
		}
		if origin != "all" && f.Origin != origin {
			continue
		}
		p := f.Folder
		if root != "" && p != root && !strings.HasPrefix(p, root+" / ") {
			continue
		}
		if p == "" {
			loose++
		}
		e, ok := folders[p]
		if !ok {
			e = &folderCount{Path: p}
			folders[p] = e
		}
		e.Count++
		if f.UpdatedAt > e.Latest {
			e.Latest = f.UpdatedAt
		}
	}
	out := make([]folderCount, 0, len(folders))
	total := 0
	for _, e := range folders {
		out = append(out, *e)
		total += e.Count
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Path < out[j].Path
	})
	writeJSON(w, 200, map[string]any{
		"origin": origin, "root": root, "total": total, "loose": loose, "folders": out,
	})
}

// ---------- /api/favorite/snapshots ----------

func (s *Server) handleFavoriteSnapshots(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.FavoriteSnapshots()
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	if list == nil {
		list = []SnapshotMeta{}
	}
	writeJSON(w, 200, map[string]any{"snapshots": list})
}

// ---------- 导入：预览 + 确认 ----------

func (s *Server) handleImportPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeErr(w, 405, "只支持 POST")
		return
	}
	var body struct {
		Data json.RawMessage `json:"data"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(&body); err != nil || len(body.Data) == 0 {
		writeErr(w, 400, "请求格式错误")
		return
	}
	res, err := NormalizeImport(body.Data)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}

	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	id := hex.EncodeToString(buf)

	previewMu.Lock()
	now := time.Now()
	for k, v := range previews {
		if v.expires.Before(now) {
			delete(previews, k)
		}
	}
	if len(previews) >= previewLimit {
		for k := range previews { // 超限时丢最早的一个（map 顺序随机，够用）
			delete(previews, k)
			break
		}
	}
	previews[id] = previewEntry{data: res, expires: now.Add(previewTTL)}
	previewMu.Unlock()

	bili, browser := 0, 0
	for _, h := range res.History {
		if h.Source == "bilibili" {
			bili++
		} else {
			browser++
		}
	}
	warnings := append([]string{}, res.Warnings...)
	warnings = append(warnings, "确认后合并；相同身份去重，删除标记阻止旧备份复活。收藏夹仅导入网址及目录路径，不反写浏览器。")
	writeJSON(w, 200, map[string]any{
		"previewId": id,
		"counts": map[string]int{
			"bilibili": bili, "browser": browser,
			"favorites": len(res.Favorites), "tombstones": len(res.Tombstones),
		},
		"warnings": warnings,
	})
}

func (s *Server) handleImportConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeErr(w, 405, "只支持 POST")
		return
	}
	var body struct {
		PreviewID string `json:"previewId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	previewMu.Lock()
	entry, ok := previews[body.PreviewID]
	if ok {
		delete(previews, body.PreviewID)
	}
	previewMu.Unlock()
	if !ok || entry.expires.Before(time.Now()) {
		writeErr(w, 409, "预览已失效，请重新选择文件")
		return
	}

	batchID := "manual-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	res, err := s.store.Ingest("manual", batchID, body.PreviewID, "manual", entry.data)
	if err != nil {
		writeErr(w, 500, "合并失败："+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"imported":   res.Imported,
		"backfilled": res.Backfilled,
		"skipped":    res.Skipped,
		"favorites":  res.Favorites,
		"snapshot":   res.Snapshot,
		"tombstones": res.Tombstones,
	})
}

// ---------- /api/backup ----------

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	history, err := s.store.AllHistory()
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	favorites, err := s.store.AllFavorites()
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	tombstones, err := s.store.Tombstones()
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	SortHistoryDesc(history)
	w.Header().Set("Content-Disposition", `attachment; filename="unlimited-history-local-backup.json"`)
	writeJSON(w, 200, map[string]any{
		"format": "unlimited-history-local-demo", "version": 1,
		"exportedAt": time.Now().UnixMilli(),
		"history":    history, "favorites": favorites, "tombstones": tombstones,
	})
}

// ---------- 小工具 ----------

func paginate(total, cursor, limit int) ([2]int, any) {
	if cursor > total {
		cursor = total
	}
	end := cursor + limit
	if end > total {
		end = total
	}
	var next any
	if end < total {
		next = end
	}
	return [2]int{cursor, end}, next
}

func orAll(s string) string {
	if s == "" {
		return "all"
	}
	return s
}
