// ban.go — 用户封禁 (F2)：登录后的写操作统一校验封禁状态
// 封禁账号：无法登录（登录已拦截）+ 无法发帖/评论/评价/上传/私信/举报/点赞等写操作
// 其内容在全站前台列表/详情/搜索中隐藏
package main

import "net/http"

// currentUserBanned 返回当前登录用户是否被封禁
func currentUserBanned(r *http.Request) bool {
	uid := currentUserID(r)
	if uid == 0 {
		return false
	}
	u, err := authStore.GetByID(uid)
	if err != nil || u == nil {
		return false
	}
	return u.Banned == 1
}
