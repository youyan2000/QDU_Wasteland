// import.go — 把各种导出 JSON 规范成统一的导入结果。
//
// 这是从 upstream unlimited_history 的 demo/server.mjs 移植而来的核心逻辑，
// 支持五种输入形态：
//  1. format=unlimited-history-local-demo v1   本工具自己的完整备份
//  2. format=unlimited-history-personal v1     个人档案（含 bilibili/browserHistory/browserBookmarks）
//  3. schemaVersion=2|3                        B站历史备份
//  4. format=browser-history-archive v1         浏览器访问历史
//  5. type=browser-bookmarks v1                 浏览器收藏夹
//
// 去重身份由这里的 id 决定，改它等于改语义：
//
//	B站历史    bili:<uid>:<business>:<contentId>:<view_at>
//	浏览器历史  browser:<deviceId>:<nativeVisitId>
//	收藏       bookmark:<deviceId>:<nativeId>
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ImportLimit 是单次导入的记录上限（与上游一致）。
const ImportLimit = 30000

var biliBusiness = map[string]bool{
	"archive": true, "pgc": true, "article": true,
	"article-list": true, "live": true, "cheese": true,
}

// ---------- 取值助手（输入是任意 map，形态由上游决定）----------

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asArr(v any) []any {
	a, _ := v.([]any)
	return a
}

func str(m map[string]any, k string) string {
	switch v := m[k].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	}
	return ""
}

func num(m map[string]any, k string) int64 {
	switch v := m[k].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	return 0
}

func boolean(m map[string]any, k string) bool {
	b, _ := m[k].(bool)
	return b
}

// cleanText 去掉控制字符并截断。
func cleanText(s string, max int) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 && r != '\t' && r != '\n' {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

func cleanURL(s string, max int) (string, error) {
	s = cleanText(s, max)
	if s == "" {
		return "", nil
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		return "", errors.New("链接格式无效")
	}
	return s, nil
}

// ---------- 入口 ----------

// NormalizeImport 解析一份导出 JSON。
func NormalizeImport(data []byte) (*ImportResult, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, errors.New("不是有效的 JSON")
	}
	out := &ImportResult{}

	switch {
	case str(root, "format") == "unlimited-history-local-demo" && num(root, "version") == 1:
		if err := passthrough(root, out); err != nil {
			return nil, err
		}
	case str(root, "format") == "unlimited-history-personal" && num(root, "version") == 1:
		if err := importBili(asMap(root["bilibili"]), out); err != nil {
			return nil, err
		}
		if err := importBrowser(asMap(root["browserHistory"]), out); err != nil {
			return nil, err
		}
		if err := importBookmarks(asMap(root["browserBookmarks"]), out); err != nil {
			return nil, err
		}
	case num(root, "schemaVersion") == 2 || num(root, "schemaVersion") == 3:
		if err := importBili(root, out); err != nil {
			return nil, err
		}
	case str(root, "format") == "browser-history-archive":
		if err := importBrowser(root, out); err != nil {
			return nil, err
		}
	case str(root, "type") == "browser-bookmarks":
		if err := importBookmarks(root, out); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("不支持的 JSON 格式（支持完整个人档案、B站 schema2/3、浏览器历史 v1、浏览器收藏夹 v1）")
	}

	if out.Total() > ImportLimit {
		return nil, fmt.Errorf("每次导入最多 %d 条，请分批导出", ImportLimit)
	}
	return out, nil
}

// ---------- 1. 本工具自己的备份（已是规范形态，只做校验与截断）----------

func passthrough(root map[string]any, out *ImportResult) error {
	for _, raw := range asArr(root["history"]) {
		item, err := canonicalHistory(asMap(raw))
		if err != nil {
			return err
		}
		out.History = append(out.History, item)
	}
	for _, raw := range asArr(root["favorites"]) {
		item, err := canonicalFavorite(asMap(raw))
		if err != nil {
			return err
		}
		out.Favorites = append(out.Favorites, item)
	}
	for _, raw := range asArr(root["tombstones"]) {
		item, err := canonicalTombstone(asMap(raw))
		if err != nil {
			return err
		}
		out.Tombstones = append(out.Tombstones, item)
	}
	return nil
}

// ---------- 2. B站历史 ----------

func importBili(backup map[string]any, out *ImportResult) error {
	if backup == nil {
		return nil
	}
	schema := num(backup, "schemaVersion")
	if schema != 2 && schema != 3 {
		return errors.New("B站历史只支持 schemaVersion 2/3")
	}

	for _, raw := range asArr(backup["events"]) {
		item := asMap(raw)
		account := int64(0)
		if schema == 3 {
			account = num(item, "uid")
		}
		business := cleanText(str(item, "business"), 30)
		if !biliBusiness[business] {
			return errors.New("B站业务类型无效")
		}
		contentID := num(item, "id")
		viewed := num(item, "view_at")
		eventKey := fmt.Sprintf("%d:%s:%d:%d", account, business, contentID, viewed)

		target := str(item, "uri")
		if target == "" {
			if bvid := str(item, "bvid"); bvid != "" {
				if !isBVID(bvid) {
					return errors.New("BVID 无效")
				}
				target = "https://www.bilibili.com/video/" + bvid
			}
		}
		if target == "" {
			if business == "live" {
				target = fmt.Sprintf("https://live.bilibili.com/%d", contentID)
			} else {
				target = fmt.Sprintf("https://www.bilibili.com/video/av%d", contentID)
			}
		}
		u, err := cleanURL(target, 4096)
		if err != nil {
			return err
		}

		h := History{
			ID:         "bili:" + eventKey,
			Source:     "bilibili",
			Title:      cleanText(str(item, "title"), 1000),
			URL:        u,
			VisitedAt:  viewed * 1000,
			UID:        account,
			Business:   business,
			ContentID:  contentID,
			EventKey:   eventKey,
			Cover:      httpsURL(cleanText(str(item, "cover"), 4096)),
			AuthorName: cleanText(str(item, "author_name"), 200),
			AuthorMid:  num(item, "author_mid"),
			Duration:   num(item, "duration"),
			Progress:   num(item, "progress"),
			IsFav:      boolean(item, "is_fav"),
			TagName:    cleanText(str(item, "tag_name"), 40),
		}
		out.History = append(out.History, h)
	}

	for _, raw := range asArr(backup["tombstones"]) {
		item := asMap(raw)
		account := int64(0)
		if schema == 3 {
			account = num(item, "uid")
		}
		deletedAt := num(item, "deleted_at")
		through := num(item, "deleted_through_view_at")

		switch str(item, "scope") {
		case "event":
			parts := strings.Split(cleanText(str(item, "event_id"), 512), ":")
			if len(parts) != 3 && len(parts) != 4 {
				return errors.New("B站事件删除键无效")
			}
			last3 := parts[len(parts)-3:]
			business, id, viewed := last3[0], last3[1], last3[2]
			if len(parts) == 4 && schema == 3 {
				if n, err := strconv.ParseInt(parts[0], 10, 64); err != nil || n != account {
					return errors.New("B站删除键 UID 不匹配")
				}
			}
			if !biliBusiness[business] || !isDigits(id) || !isDigits(viewed) {
				return errors.New("B站事件删除键无效")
			}
			out.Tombstones = append(out.Tombstones, Tombstone{
				Kind: "history", ID: fmt.Sprintf("bili:%d:%s:%s:%s", account, business, id, viewed), DeletedAt: deletedAt,
			})
		case "content":
			parts := strings.Split(cleanText(str(item, "content_key"), 512), ":")
			if len(parts) != 2 && len(parts) != 3 {
				return errors.New("B站内容删除键无效")
			}
			last := parts[len(parts)-2:]
			business, id := last[0], last[1]
			if len(parts) == 3 && schema == 3 {
				if n, err := strconv.ParseInt(parts[0], 10, 64); err != nil || n != account {
					return errors.New("B站删除键 UID 不匹配")
				}
			}
			if !biliBusiness[business] || !isDigits(id) {
				return errors.New("B站内容删除键无效")
			}
			cid, _ := strconv.ParseInt(id, 10, 64)
			out.Tombstones = append(out.Tombstones, Tombstone{
				Kind: "bilibili-content", ID: fmt.Sprintf("%d:%s:%s", account, business, id),
				UID: account, Business: business, ContentID: cid, Through: through, DeletedAt: deletedAt,
			})
		case "legacy-id":
			contentID := num(item, "legacy_id")
			out.Tombstones = append(out.Tombstones, Tombstone{
				Kind: "bilibili-legacy", ID: fmt.Sprintf("%d:%d", account, contentID),
				UID: account, ContentID: contentID, Through: through, DeletedAt: deletedAt,
			})
		default:
			return errors.New("未知的 B站删除标记")
		}
	}

	if schema == 2 {
		out.Warnings = append(out.Warnings, "旧版 B站 schema2 不含可信账号归属，统一归 UID 0（未归属）。")
	}
	return nil
}

// ---------- 3. 浏览器历史 ----------

func importBrowser(backup map[string]any, out *ImportResult) error {
	if backup == nil {
		return nil
	}
	if str(backup, "format") != "browser-history-archive" || num(backup, "version") != 1 {
		return errors.New("不是浏览器历史 v1 备份")
	}
	names := map[string]string{}
	for _, raw := range asArr(backup["devices"]) {
		d := asMap(raw)
		names[cleanText(str(d, "id"), 512)] = cleanText(str(d, "name"), 200)
	}
	for _, raw := range asArr(backup["visits"]) {
		item := asMap(raw)
		deviceID := cleanText(str(item, "deviceId"), 512)
		native := cleanText(str(item, "nativeVisitId"), 512)
		if str(item, "id") != deviceID+":"+native {
			return errors.New("浏览器访问身份不匹配")
		}
		u, err := cleanURL(str(item, "url"), 4096)
		if err != nil {
			return err
		}
		name := names[deviceID]
		if name == "" {
			name = deviceID
		}
		out.History = append(out.History, History{
			ID: "browser:" + deviceID + ":" + native, Source: "browser",
			Title: cleanText(str(item, "title"), 1000), URL: u,
			VisitedAt: num(item, "visitTime"), DeviceID: deviceID, DeviceName: name,
		})
	}
	for _, raw := range asArr(backup["tombstones"]) {
		item := asMap(raw)
		out.Tombstones = append(out.Tombstones, Tombstone{
			Kind: "history", ID: "browser:" + cleanText(str(item, "id"), 512), DeletedAt: num(item, "deletedAt"),
		})
	}
	return nil
}

// ---------- 4. 浏览器收藏夹 ----------

func importBookmarks(backup map[string]any, out *ImportResult) error {
	if backup == nil {
		return nil
	}
	if str(backup, "type") != "browser-bookmarks" || num(backup, "version") != 1 {
		return errors.New("不是浏览器收藏夹 v1 备份")
	}
	rows := asArr(backup["records"])
	folders := map[string]map[string]any{}
	for _, raw := range rows {
		item := asMap(raw)
		device := cleanText(str(item, "deviceId"), 512)
		native := cleanText(str(item, "nativeId"), 512)
		kind := str(item, "kind")
		if str(item, "id") != device+":"+native || (kind != "folder" && kind != "bookmark" && kind != "separator") {
			return errors.New("浏览器收藏节点身份无效")
		}
		folders[str(item, "id")] = item
	}
	for _, raw := range rows {
		item := asMap(raw)
		if str(item, "kind") != "bookmark" {
			continue
		}
		var path []string
		parent := cleanText(str(item, "parentId"), 512)
		seen := map[string]bool{str(item, "nativeId"): true}
		device := cleanText(str(item, "deviceId"), 512)
		for parent != "" && !seen[parent] {
			seen[parent] = true
			folder, ok := folders[device+":"+parent]
			if !ok {
				break
			}
			if title := cleanText(str(folder, "title"), 1000); title != "" {
				path = append([]string{title}, path...)
			}
			parent = cleanText(str(folder, "parentId"), 512)
			if len(path) > 64 {
				return errors.New("书签目录嵌套超过 64 层")
			}
		}
		u, err := cleanURL(str(item, "url"), 4096)
		if err != nil {
			return err
		}
		native := cleanText(str(item, "nativeId"), 512)
		out.Favorites = append(out.Favorites, Favorite{
			ID: "bookmark:" + device + ":" + native, Origin: "browser",
			Title: cleanText(str(item, "title"), 1000), URL: u, Tags: []string{},
			DeviceID: device, DeviceName: cleanText(str(item, "deviceName"), 200),
			Folder: strings.Join(path, " / "), UpdatedAt: num(item, "updatedAt"),
			DeletedAt: num(item, "deletedAt"),
		})
	}
	return nil
}

// ---------- 校验助手 ----------

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isBVID(s string) bool {
	if !strings.HasPrefix(s, "BV") || len(s) < 3 {
		return false
	}
	for i := 2; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// httpsURL 把封面等外链统一升级到 https：站点 CSP 只允许 https 图片，
// B站 旧数据里常见 http:// 或 // 开头的地址，直接渲染会被浏览器拦掉。
func httpsURL(s string) string {
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	if strings.HasPrefix(s, "http://") {
		return "https://" + strings.TrimPrefix(s, "http://")
	}
	return s
}

// canonicalHistory 用于「本工具自己的备份」这条路径。
func canonicalHistory(item map[string]any) (History, error) {
	source := str(item, "source")
	if source != "bilibili" && source != "browser" {
		return History{}, errors.New("未知的历史来源")
	}
	u, err := cleanURL(str(item, "url"), 4096)
	if err != nil {
		return History{}, err
	}
	h := History{
		ID: cleanText(str(item, "id"), 512), Source: source,
		Title: cleanText(str(item, "title"), 1000), URL: u, VisitedAt: num(item, "visited_at"),
	}
	if source == "bilibili" {
		h.UID = num(item, "uid")
		h.Business = cleanText(str(item, "business"), 30)
		h.ContentID = num(item, "contentId")
		h.EventKey = cleanText(str(item, "eventKey"), 512)
		h.Cover = httpsURL(cleanText(str(item, "cover"), 4096))
		h.AuthorName = cleanText(str(item, "authorName"), 200)
		h.AuthorMid = num(item, "authorMid")
		h.Duration = num(item, "duration")
		h.Progress = num(item, "progress")
		h.IsFav = boolean(item, "isFav")
		h.TagName = cleanText(str(item, "tagName"), 40)
	} else {
		h.DeviceID = cleanText(str(item, "deviceId"), 512)
		h.DeviceName = cleanText(str(item, "deviceName"), 200)
	}
	if h.ID == "" {
		return History{}, errors.New("历史记录缺少 id")
	}
	return h, nil
}

func canonicalFavorite(item map[string]any) (Favorite, error) {
	origin := str(item, "origin")
	if origin != "browser" && origin != "manual" {
		return Favorite{}, errors.New("未知的收藏来源")
	}
	u, err := cleanURL(str(item, "url"), 4096)
	if err != nil {
		return Favorite{}, err
	}
	f := Favorite{
		ID: cleanText(str(item, "id"), 512), Origin: origin,
		Title: cleanText(str(item, "title"), 1000), URL: u,
		Note:     cleanText(str(item, "note"), 4096),
		DeviceID: cleanText(str(item, "deviceId"), 200), DeviceName: cleanText(str(item, "deviceName"), 200),
		Folder: cleanText(str(item, "folder"), 4096), UpdatedAt: num(item, "updated_at"),
		DeletedAt: num(item, "deletedAt"),
	}
	for _, t := range asArr(item["tags"]) {
		if s, ok := t.(string); ok {
			f.Tags = append(f.Tags, cleanText(s, 100))
		}
	}
	if f.ID == "" {
		return Favorite{}, errors.New("收藏缺少 id")
	}
	return f, nil
}

func canonicalTombstone(item map[string]any) (Tombstone, error) {
	kind := str(item, "kind")
	switch kind {
	case "history", "favorite", "bilibili-content", "bilibili-legacy":
	default:
		return Tombstone{}, errors.New("未知的删除标记")
	}
	t := Tombstone{Kind: kind, ID: cleanText(str(item, "id"), 512), DeletedAt: num(item, "deletedAt")}
	if strings.HasPrefix(kind, "bilibili-") {
		t.UID = num(item, "uid")
		t.ContentID = num(item, "contentId")
		t.Business = cleanText(str(item, "business"), 30)
		t.Through = num(item, "through")
	}
	if t.ID == "" {
		return Tombstone{}, errors.New("删除标记缺少 id")
	}
	return t, nil
}
