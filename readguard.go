// readguard.go — 读取侧可见性统一判定（审计 P1-4）
//
// 早期实现只在列表接口过滤状态，详情/下载/预览这些"按 ID 直接取"的路径放行了
// 「待复核」「已下架」的内容：ID 是自增的，于是被判违规的资料反而拿到了更稳定的外链。
// 这里把规则收敛成两个函数，所有读取路径都必须过它们。
package main

import "net/http"

// contentVisibleTo 判断某条内容能否被当前请求者读取。
//   - 状态为空或「正常」：所有人可见
//   - 「draft / 待复核 / 已下架 / 已驳回」：仅作者本人与管理员可见
//   - 作者被封禁：对外一律不可见
func contentVisibleTo(r *http.Request, status string, ownerID int64, authorBanned int) bool {
	if authorBanned == 1 {
		return false
	}
	switch status {
	case "", "正常":
		return true
	}
	me := currentUserID(r)
	if me == 0 {
		return false
	}
	if me == ownerID {
		return true
	}
	u, err := authStore.GetByID(me)
	return err == nil && u != nil && u.IsAdmin == 1
}

// fileReadableBy 是资料文件（下载/预览）的可见性判定。
func fileReadableBy(r *http.Request, fileID int64, status string) bool {
	if status == "" || status == "正常" {
		return true
	}
	var owner int64
	_ = authStore.db.QueryRow("SELECT COALESCE(user_id,0) FROM files WHERE id=?", fileID).Scan(&owner)
	return contentVisibleTo(r, status, owner, 0)
}
