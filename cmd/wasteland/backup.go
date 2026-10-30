// backup.go — H5 数据自动备份
// POST /api/admin/backup  管理员手动或定时触发：
//   1. SQLite 一致性快照（VACUUM INTO 生成独立完整库文件，WAL 安全）
//   2. 打包 uploads/ 上传目录为 zip
//   3. 轮转：只保留最近 backupKeep 份，自动清理旧备份
// 定时执行：部署机 cron / Windows 计划任务调用本接口即可（见 deploy/backup.sh、backup.ps1）。
package main

import (
	"archive/zip"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const backupDir = "backups" // 备份输出目录（已加入 .gitignore）
const backupKeep = 14       // 保留最近 14 份备份

// handleAdminBackup 触发一次完整备份（管理员）
func handleAdminBackup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(authStore, w, r) == 0 {
			return
		}
		if r.Method != http.MethodPost {
			apiErr(w, 405, "仅支持 POST"); return
		}
		ts := time.Now().Format("2006-01-02-150405")
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			apiErr(w, 500, "备份目录创建失败"); return
		}
		// 1) SQLite 一致性快照（VACUUM INTO 生成独立完整库文件）
		dbSnap := filepath.Join(backupDir, "qdu-"+ts+".db")
		// 路径为内部生成（时间戳），无用户输入，拼接进 SQL 安全
		_, err := authStore.db.Exec("VACUUM INTO '" + strings.ReplaceAll(dbSnap, "'", "''") + "'")
		if err != nil {
			apiErr(w, 500, "数据库快照失败: "+err.Error()); return
		}
		// 2) 打包 uploads/（存在时）
		uploadsZip := filepath.Join(backupDir, "uploads-"+ts+".zip")
		if _, statErr := os.Stat("uploads"); statErr == nil {
			if zipErr := zipDir("uploads", uploadsZip); zipErr != nil {
				apiErr(w, 500, "上传目录打包失败"); return
			}
		}
		// 3) 轮转：只保留最近 backupKeep 份
		pruned := pruneBackups()
		// 4) 统计
		var size int64
		if fi, e := os.Stat(dbSnap); e == nil { size += fi.Size() }
		if fi, e := os.Stat(uploadsZip); e == nil { size += fi.Size() }
		logAudit(authStore.db, currentUserID(r), "数据备份", "snapshot="+ts+" size="+strconv.FormatInt(size, 10)+" pruned="+strconv.Itoa(pruned))
		apiJSON(w, 200, map[string]any{"ok": true, "time": ts, "db": dbSnap, "uploads": uploadsZip, "size": size, "pruned": pruned})
	}
}

// zipDir 递归压缩目录
func zipDir(srcDir, zipPath string) error {
	f, err := os.Create(zipPath)
	if err != nil { return err }
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil { return err }
		if info.IsDir() { return nil }
		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil { return relErr }
		zh, zhErr := zw.Create(filepath.ToSlash(rel))
		if zhErr != nil { return zhErr }
		src, openErr := os.Open(path)
		if openErr != nil { return openErr }
		defer src.Close()
		_, copyErr := io.Copy(zh, src)
		return copyErr
	})
}

// pruneBackups 删除超过保留份数的旧备份（db 与同名批次 zip 一起删），返回删除批次数
func pruneBackups() int {
	entries, err := os.ReadDir(backupDir)
	if err != nil { return 0 }
	var batches []string
	for _, e := range entries {
		if e.IsDir() { continue }
		name := e.Name()
		var ts string
		if strings.HasPrefix(name, "qdu-") {
			ts = strings.TrimSuffix(strings.TrimPrefix(name, "qdu-"), ".db")
		} else if strings.HasPrefix(name, "uploads-") {
			ts = strings.TrimSuffix(strings.TrimPrefix(name, "uploads-"), ".zip")
		}
		if ts != "" && !containsStr(batches, ts) {
			batches = append(batches, ts)
		}
	}
	sort.Strings(batches)
	pruned := 0
	for i := 0; i < len(batches)-backupKeep; i++ {
		_ = os.Remove(filepath.Join(backupDir, "qdu-"+batches[i]+".db"))
		_ = os.Remove(filepath.Join(backupDir, "uploads-"+batches[i]+".zip"))
		pruned++
	}
	return pruned
}

