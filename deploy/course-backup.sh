#!/usr/bin/env bash
# course-backup.sh — 课程资料备份（方向②：服务器 → GitHub QDU_course_resource）
# 功能：
#   1. 从 SQLite 读取 files 表（正常状态），导出"文件→课程"清单（匿名：不含上传者信息）
#   2. 按课程编号把 uploads/ 里的文件复制到 staging 目录（跳过 >100MB 大文件，GitHub 限制）
#   3. 生成 file-list.csv（课程编号/标题/文件名/分类/学期/大小/时间）
#   4. git commit + push 到 https://github.com/youyan2000/QDU_course_resource.git
# 定时：crontab -e 加一行（每天 4 点）：
#   QW_GITHUB_TOKEN=xxx /opt/qdu-wasteland/deploy/course-backup.sh >> /var/log/qdu-course-backup.log 2>&1
set -euo pipefail

# ---------- 配置 ----------
APP_DIR="${QW_APP_DIR:-/root/qdu-wasteland}"      # 应用目录
UPLOADS_DIR="$APP_DIR/uploads"                     # 上传文件目录
DB_FILE="$APP_DIR/qdu-auth.db"                     # SQLite 数据库
STAGING="${QW_STAGING:-/tmp/qdu-course-staging}"   # 暂存目录
REPO_URL="https://github.com/youyan2000/QDU_course_resource.git"
MAX_SIZE="$((100*1024*1024))"                      # GitHub 单文件上限 100MB
GITHUB_TOKEN="${QW_GITHUB_TOKEN:?请设置 QW_GITHUB_TOKEN（GitHub Personal Access Token，repo 权限）}"
GIT_USER="${QW_GIT_USER:-qdu-course-backup}"
GIT_EMAIL="${QW_GIT_EMAIL:-backup@qdu.local}"

# ---------- 1. 准备 ----------
echo "==> [1/5] 准备 staging 目录"
rm -rf "$STAGING"
mkdir -p "$STAGING/files"

# ---------- 2. 导出清单 + 复制文件 ----------
echo "==> [2/5] 从数据库导出清单并复制文件"
# 检查 sqlite3 是否可用
if ! command -v sqlite3 >/dev/null 2>&1; then
  echo "❌ 需要 sqlite3 命令（apt install sqlite3）"; exit 1
fi

# 导出 CSV（仅正常状态；不含 uploader_id/is_anonymous 等个人信息）
sqlite3 -separator $'\t' "$DB_FILE" "
  SELECT course_code, COALESCE(title,''), file_name, COALESCE(category,''),
         COALESCE(semester,''), COALESCE(description,''), size, created_at
  FROM files WHERE status='正常' ORDER BY course_code, id;
" > "$STAGING/file-list.tsv" || { echo "❌ 数据库读取失败"; exit 1; }

# 逐行处理：复制文件 + 写 CSV 头
{
  echo "course_code,title,file_name,category,semester,description,size,created_at"
  skip_big=0
  copied=0
  while IFS=$'\t' read -r code title fname cat sem desc size created; do
    [ -z "$code" ] && continue
    # 找到源文件：stored_name 是 f_时间戳_id.扩展名，需从数据库取
    stored=$(sqlite3 "$DB_FILE" "SELECT stored_name FROM files WHERE course_code='$code' AND file_name='$fname' AND status='正常' LIMIT 1;" 2>/dev/null || true)
    [ -z "$stored" ] && stored="$fname"
    src="$UPLOADS_DIR/$stored"
    if [ ! -f "$src" ]; then
      # 尝试按 file_name 直接找（某些部署 stored_name=file_name）
      src="$UPLOADS_DIR/$fname"
    fi
    if [ ! -f "$src" ]; then
      echo "  ⚠️ 文件缺失: $stored ($fname)" >&2
      continue
    fi
    # 跳过 >100MB（GitHub 限制），记录到清单但不复制
    size_bytes=$(stat -c%s "$src" 2>/dev/null || echo "$size")
    if [ "$size_bytes" -gt "$MAX_SIZE" ]; then
      echo "  ⏭️ 跳过大文件(>100MB): $fname ($((size_bytes/1024/1024))MB)" >&2
      skip_big=$((skip_big+1))
      continue
    fi
    # 按课程分文件夹复制
    mkdir -p "$STAGING/files/$code"
    cp "$src" "$STAGING/files/$code/$fname" 2>/dev/null || { echo "  ⚠️ 复制失败: $fname" >&2; continue; }
    copied=$((copied+1))
    # 转义 CSV（引号包裹含逗号/引号的字段）
    csv_escape() { printf '%s' "$1" | sed 's/"/""/g'; }
    echo "\"$(csv_escape "$code")\",\"$(csv_escape "$title")\",\"$(csv_escape "$fname")\",\"$(csv_escape "$cat")\",\"$(csv_escape "$sem")\",\"$(csv_escape "$desc")\",$size_bytes,\"$created\""
  done < "$STAGING/file-list.tsv"
} > "$STAGING/file-list.csv"
echo "  复制 $copied 个文件，跳过大文件 $skip_big 个"
rm -f "$STAGING/file-list.tsv"

# ---------- 3. 生成 README ----------
echo "==> [3/5] 生成 README"
cat > "$STAGING/README.md" <<'EOF'
# 课程资料备份（匿名化公开版）

> 自动备份：从服务器上传资料导出（不含任何上传者个人信息）。
> 由 `deploy/course-backup.sh` 定时生成并推送。

## 内容
- `files/` — 按课程编号分文件夹的资料文件（≤100MB）
- `file-list.csv` — 文件清单（课程编号/标题/文件名/分类/学期/描述/大小/时间）

## 说明
- 超过 100MB 的文件因 GitHub 限制不在此仓库（由服务器本地完整备份兜底）。
- 上传者信息（昵称/邮箱）已剔除，仅站长服务器数据库保留。
EOF

# ---------- 4. git 提交 ----------
echo "==> [4/5] git commit"
cd "$STAGING"
git init -q 2>/dev/null || true
git config user.name "$GIT_USER"
git config user.email "$GIT_EMAIL"
git remote remove origin 2>/dev/null || true
git remote add origin "$REPO_URL"
git add -A
if git diff --cached --quiet; then
  echo "  无新改动，跳过提交"
else
  git commit -q -m "课程资料备份 $(date +%Y-%m-%d_%H%M%S)"
fi

# ---------- 5. 推送 ----------
echo "==> [5/5] push 到 GitHub"
# 用 token 推送（token 只授权此仓库）
git push -q "https://x-access-token:${GITHUB_TOKEN}@github.com/youyan2000/QDU_course_resource.git" main 2>&1 || {
  # 若远程为空仓库且无 main，先建 main 分支
  git branch -M main 2>/dev/null || true
  git push -q "https://x-access-token:${GITHUB_TOKEN}@github.com/youyan2000/QDU_course_resource.git" main
}
echo "✅ 完成: $(date)"
