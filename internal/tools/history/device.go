// device.go — 浏览器扩展配对：设备令牌、自动同步接口、扩展打包下载。
//
// 设计要点：
//   - 令牌只在生成时显示一次，库里只存 SHA-256；
//   - 每个设备可单独撤销；
//   - 同步请求用 Bearer 令牌鉴权，不依赖网页会话 Cookie，
//     因此扩展可以在后台定时推送，不需要用户每次打开网页。
package history

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed extension
var extensionFS embed.FS

// maxDevicesPerUser 限制单个账号的活跃配对设备数。
const maxDevicesPerUser = 10

const deviceSchema = `
CREATE TABLE IF NOT EXISTS history_devices (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL,
  label      TEXT NOT NULL DEFAULT '',
  token_hash TEXT NOT NULL UNIQUE,
  created_at INTEGER NOT NULL,
  last_sync  INTEGER NOT NULL DEFAULT 0,
  revoked    INTEGER NOT NULL DEFAULT 0
);`

// Device 是配对设备的展示信息。
type Device struct {
	ID        int64  `json:"id"`
	Label     string `json:"label"`
	CreatedAt int64  `json:"createdAt"`
	LastSync  int64  `json:"lastSync"`
}

func ensureDeviceSchema(db *sql.DB) error {
	_, err := db.Exec(deviceSchema)
	return err
}

func hashDeviceToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// IssueDeviceToken 生成一次性可见的配对令牌。
func IssueDeviceToken(db *sql.DB, userID int64, label string) (string, int64, error) {
	if db == nil || userID <= 0 {
		return "", 0, errors.New("无效账号")
	}
	if err := ensureDeviceSchema(db); err != nil {
		return "", 0, err
	}
	// 每个账号最多 10 台设备，防止无限生成配对码把表撑大。
	var active int
	if err := db.QueryRow(`SELECT COUNT(*) FROM history_devices WHERE user_id=? AND revoked=0`, userID).Scan(&active); err != nil {
		return "", 0, err
	}
	if active >= maxDevicesPerUser {
		return "", 0, errors.New("配对设备已达上限（10 台），请先在列表里撤销不用的设备")
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", 0, err
	}
	token := "qw_" + hex.EncodeToString(buf)
	label = strings.TrimSpace(label)
	if label == "" {
		label = "浏览器扩展"
	}
	if len(label) > 60 {
		label = label[:60]
	}
	res, err := db.Exec(`INSERT INTO history_devices(user_id,label,token_hash,created_at) VALUES(?,?,?,?)`,
		userID, label, hashDeviceToken(token), time.Now().UnixMilli())
	if err != nil {
		return "", 0, err
	}
	id, _ := res.LastInsertId()
	return token, id, nil
}

// ListDevices 返回当前账号的配对设备（不含令牌）。
func ListDevices(db *sql.DB, userID int64) ([]Device, error) {
	if err := ensureDeviceSchema(db); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id,label,created_at,last_sync FROM history_devices WHERE user_id=? AND revoked=0 ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.Label, &d.CreatedAt, &d.LastSync); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RevokeDevice 撤销一个设备，之后该令牌无法再同步。
func RevokeDevice(db *sql.DB, userID, id int64) error {
	_, err := db.Exec(`UPDATE history_devices SET revoked=1 WHERE id=? AND user_id=?`, id, userID)
	return err
}

// deviceByToken 用令牌换取账号与设备 ID。
func deviceByToken(db *sql.DB, token string) (int64, int64, bool, error) {
	if !strings.HasPrefix(token, "qw_") {
		return 0, 0, false, nil
	}
	var userID, id int64
	err := db.QueryRow(`SELECT user_id,id FROM history_devices WHERE token_hash=? AND revoked=0`, hashDeviceToken(token)).
		Scan(&userID, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	return userID, id, true, nil
}

// SyncHandler 处理扩展的自动同步：Bearer 令牌 + 一批规范化的历史数据。
//
// 请求体格式与站点自己的备份一致，方便复用同一套校验：
//
//	{"batchId":"2026-11-03T13:00:00Z","data":{"format":"unlimited-history-local-demo","version":1,"history":[...],"favorites":[...]}}
func SyncHandler(db *sql.DB, maxBytes int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON := func(status int, v any) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(v)
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(http.StatusMethodNotAllowed, map[string]string{"error": "只支持 POST"})
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeJSON(http.StatusUnauthorized, map[string]string{"error": "缺少设备令牌"})
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		userID, deviceID, ok, err := deviceByToken(db, token)
		if err != nil {
			writeJSON(http.StatusInternalServerError, map[string]string{"error": "校验失败"})
			return
		}
		if !ok {
			writeJSON(http.StatusUnauthorized, map[string]string{"error": "设备令牌无效或已撤销"})
			return
		}

		var body struct {
			BatchID string          `json:"batchId"`
			Data    json.RawMessage `json:"data"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
		if err := dec.Decode(&body); err != nil || len(body.Data) == 0 {
			writeJSON(http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
			return
		}
		res, err := NormalizeImport(body.Data)
		if err != nil {
			writeJSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		batch := strings.TrimSpace(body.BatchID)
		if batch == "" {
			batch = "ext-" + time.Now().UTC().Format("20060102T150405")
		}
		result, err := NewStoreMust(db, userID).Ingest("extension",
			batch, hashDeviceToken(string(body.Data))[:16], "extension", res)
		if err != nil {
			writeJSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		_, _ = db.Exec(`UPDATE history_devices SET last_sync=? WHERE id=?`, time.Now().UnixMilli(), deviceID)

		writeJSON(http.StatusOK, map[string]any{
			"ok":        true,
			"duplicate": result.Duplicate,
			"imported":  result.Imported,
			"favorites": result.Favorites,
			"deviceId":  deviceID,
		})
	}
}

// NewStoreMust 返回绑定到指定账号的存储，schema 已就绪。
func NewStoreMust(db *sql.DB, userID int64) *Store {
	st := &Store{db: db, userID: userID}
	return st
}

// ExtensionZip 把内置的扩展源码打成 zip，供用户下载后"加载已解压的扩展"。
// browser 取 "firefox" 时使用 Firefox 清单（background.scripts + gecko 设置），
// 其余情况使用 Chrome/Edge 清单（MV3 service worker）。
func ExtensionZip(browser string) ([]byte, error) {
	manifest := "manifest.chrome.json"
	if strings.EqualFold(strings.TrimSpace(browser), "firefox") {
		manifest = "manifest.firefox.json"
	}
	manifestBody, err := extensionFS.ReadFile("extension/" + manifest)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("manifest.json")
	if err != nil {
		return nil, err
	}
	if _, err = f.Write(manifestBody); err != nil {
		return nil, err
	}
	err = fs.WalkDir(extensionFS, "extension", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := strings.TrimPrefix(path, "extension/")
		if strings.HasPrefix(name, "manifest.") {
			return nil // 清单由上面统一写入为 manifest.json
		}
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		src, err := extensionFS.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
