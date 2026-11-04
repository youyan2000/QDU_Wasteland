// Package tools 定义工具中心的统一扩展点。
//
// 约定：一个工具 = 一个 Go 包，实现 Tool 接口并在 init() 里 Register。
// 主程序只调用一次 Mount，新增工具不需要改动任何主程序代码。
package tools

import (
	"database/sql"
	"net/http"
)

// Access 决定谁能访问一个工具。
type Access int

const (
	// Public 对所有访客开放。
	Public Access = iota
	// OwnerOnly 仅站长可用；对其他人一律 404，不泄露存在性。
	OwnerOnly
	// Member 要求有效登录会话。
	Member
)

// String 返回稳定的标识，用于清单页与日志。
func (a Access) String() string {
	switch a {
	case OwnerOnly:
		return "owner"
	case Member:
		return "member"
	default:
		return "public"
	}
}

// Meta 是工具对外暴露的全部元信息。
type Meta struct {
	// Slug 是 URL 段，形如 "history" → /tool/history/。
	Slug string
	// Title、Summary、Label 用于清单页卡片。
	Title   string
	Summary string
	Label   string
	// Access 决定可见性。
	Access Access
	// Enabled 为 false 时清单页显示“准备中”，且不挂任何路由。
	Enabled bool
}

// Env 是主程序交给工具的运行时依赖。
// 工具不要自己去打开数据库或读进程环境，一律从这里取。
type Env struct {
	// DB 是站点唯一的 SQLite 连接（单库），工具用表前缀隔离。
	DB *sql.DB
	// DataDir 是只读/半只读数据目录（封面图、书签 JSON、归档批次）。
	DataDir string
	// OwnerUID 是站长用户 ID，0 表示未配置。
	OwnerUID int64
	// BundleRoot 是外部静态包根目录（如 NeXTJLC 的构建产物），工具可自行决定是否使用。
	BundleRoot string
	// CanOwner 判定当前请求是否来自站长；为 nil 时视为“无人是站长”。
	CanOwner func(*http.Request) bool
	// UserID 返回当前登录用户 ID；未登录或无效会话返回 0。
	UserID func(*http.Request) int64
}

// userID 统一收敛 nil 判断。
func (e Env) userID(r *http.Request) int64 {
	if e.UserID == nil {
		return 0
	}
	return e.UserID(r)
}

// isOwner 统一收敛 nil 判断，避免各工具重复写。
func (e Env) isOwner(r *http.Request) bool {
	return e.CanOwner != nil && e.CanOwner(r)
}

// Tool 是一个工具的实现契约。
//
// Register 收到的 mux 已经挂在 /tool/<slug>/ 之下，因此工具内部只写相对路径：
//
//	mux.HandleFunc("/api/stats", ...)      → /tool/<slug>/api/stats
//	mux.Handle("/", tools.Static{...})     → /tool/<slug>/ 下的静态文件
type Tool interface {
	Meta() Meta
	Register(mux *http.ServeMux, env Env)
}
