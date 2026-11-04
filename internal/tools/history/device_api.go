package history

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// handleDevices 管理扩展配对设备：列出 / 生成配对码 / 撤销。
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	uid := s.userID(r)
	if uid <= 0 {
		writeErr(w, http.StatusUnauthorized, "请先登录")
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := ListDevices(s.store.db, uid)
		if err != nil {
			writeErr(w, 500, "读取失败")
			return
		}
		writeJSON(w, 200, map[string]any{"devices": list, "now": time.Now().UnixMilli()})
	case http.MethodPost:
		var body struct {
			Label string `json:"label"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body)
		token, id, err := IssueDeviceToken(s.store.db, uid, body.Label)
		if err != nil {
			writeErr(w, 500, "生成配对码失败")
			return
		}
		writeJSON(w, 200, map[string]any{"token": token, "id": id})
	case http.MethodDelete:
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil || id <= 0 {
			writeErr(w, 400, "设备编号无效")
			return
		}
		if err := RevokeDevice(s.store.db, uid, id); err != nil {
			writeErr(w, 500, "撤销失败")
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		writeErr(w, 405, "不支持的方法")
	}
}

// handleExtensionZip 现场打包扩展源码，避免仓库里放二进制压缩包。
func (s *Server) handleExtensionZip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeErr(w, http.StatusMethodNotAllowed, "只支持 GET")
		return
	}
	browser := r.URL.Query().Get("browser")
	data, err := ExtensionZip(browser)
	if err != nil {
		writeErr(w, 500, "打包失败")
		return
	}
	name := "wasteland-history-sync-chrome.zip"
	if browser == "firefox" {
		name = "wasteland-history-sync-firefox.zip"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data)
}
