// history 工具的数据模型。
//
// 字段名与上游 unlimited_history 导出的 JSON 保持一致（camelCase 与 snake_case 混用
// 是它的既有格式，不要"顺手统一"，否则前端与旧备份都会对不上）。
package history

// History 是一条历史记录：B站观看历史或浏览器访问记录。
type History struct {
	ID        string `json:"id"`
	Source    string `json:"source"` // bilibili | browser | site
	Title     string `json:"title"`
	URL       string `json:"url"`
	VisitedAt int64  `json:"visited_at"` // 毫秒

	// B站专有
	UID        int64  `json:"uid,omitempty"`
	Business   string `json:"business,omitempty"`
	ContentID  int64  `json:"contentId,omitempty"`
	EventKey   string `json:"eventKey,omitempty"`
	Cover      string `json:"cover,omitempty"`
	AuthorName string `json:"authorName,omitempty"`
	AuthorMid  int64  `json:"authorMid,omitempty"`
	Duration   int64  `json:"duration,omitempty"`
	Progress   int64  `json:"progress,omitempty"`
	IsFav      bool   `json:"isFav,omitempty"`
	TagName    string `json:"tagName,omitempty"`

	// 浏览器专有
	DeviceID   string `json:"deviceId,omitempty"`
	DeviceName string `json:"deviceName,omitempty"`
}

// Favorite 是一条收藏（浏览器书签或手工收藏）。
type Favorite struct {
	ID         string   `json:"id"`
	Origin     string   `json:"origin"` // browser | manual
	Title      string   `json:"title"`
	URL        string   `json:"url"`
	Note       string   `json:"note,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	DeviceID   string   `json:"deviceId,omitempty"`
	DeviceName string   `json:"deviceName,omitempty"`
	Folder     string   `json:"folder,omitempty"`
	UpdatedAt  int64    `json:"updated_at"`
	DeletedAt  int64    `json:"deletedAt,omitempty"`
}

// Tombstone 是删除标记：随归档同步，防止旧备份把删掉的记录复活。
type Tombstone struct {
	Kind      string `json:"kind"` // history | favorite | bilibili-content | bilibili-legacy
	ID        string `json:"id"`
	DeletedAt int64  `json:"deletedAt"`

	// 仅 bilibili-content / bilibili-legacy 使用
	UID       int64  `json:"uid,omitempty"`
	ContentID int64  `json:"contentId,omitempty"`
	Business  string `json:"business,omitempty"`
	Through   int64  `json:"through,omitempty"`
}

// ImportResult 是一次导入规范化后的结果。
type ImportResult struct {
	History    []History
	Favorites  []Favorite
	Tombstones []Tombstone
	Warnings   []string
}

// Total 返回本次导入的记录总数（用于限额判断）。
func (r *ImportResult) Total() int {
	return len(r.History) + len(r.Favorites) + len(r.Tombstones)
}
