#!/usr/bin/env bash
# backup.sh — 服务端本地数据备份（方案C：不依赖 Web 服务、不需要任何登录凭据）
#
# 用法：
#   手动：bash /root/qdu-wasteland/deploy/backup.sh
#   定时：crontab -e 加一行（无需任何密码）
#         0 3 * * * /root/qdu-wasteland/deploy/backup.sh >> /var/log/qdu-backup.log 2>&1
#
# 为什么用本地方式而不是调 HTTP 接口：
#   1) 无需给管理员账号开"跳过验证码/限流"的后门（消除管理员账号暴力破解漏洞）
#   2) Web 服务挂了也能备份（备份比网站本身更重要）
#
# 做什么：
#   1. sqlite3 VACUUM INTO 生成一致性快照
#      —— WAL 安全：即使数据还在 qdu-auth.db-wal 里，快照也完整
#   2. 打包 uploads/ 上传目录
#   3. 自动验证快照（完整性 ok + 表数量 + 关键表行数与源库一致）—— 防止"备份了个空库"
#   4. 轮转：只保留最近 KEEP 份
# 只读取源库，绝不修改任何线上数据。
set -euo pipefail

RUN="${QW_RUN_DIR:-/root/qdu-wasteland}"
DB="${QW_DB:-$RUN/qdu-auth.db}"
BACKUP_DIR="${QW_BACKUP_DIR:-$RUN/backups}"
KEEP="${QW_BACKUP_KEEP:-14}"
STAMP="$(date '+%Y-%m-%d-%H%M%S')"

say() { echo "[$(date '+%F %T')] $*"; }
die() { echo "[$(date '+%F %T')] ✗ $*" >&2; exit 1; }

say "===== 开始备份 ($STAMP) ====="

# ---------- 0. 环境检查 ----------
command -v sqlite3 >/dev/null 2>&1 || die "未安装 sqlite3（执行: apt install -y sqlite3）"
if [ ! -f "$DB" ]; then die "找不到数据库文件 $DB"; fi
mkdir -p "$BACKUP_DIR"

# 磁盘空间检查（需要 源库+uploads 的 3 倍余量，另加 200MB 缓冲）
DB_KB=$(( $(stat -c%s "$DB") / 1024 ))
UP_KB=0
if [ -d "$RUN/uploads" ]; then
  UP_KB="$(du -sk "$RUN/uploads" 2>/dev/null | awk '{print $1+0}')"
  UP_KB="${UP_KB:-0}"
fi
NEED_KB=$(( (DB_KB + UP_KB) * 3 + 204800 ))
AVAIL_KB="$(df -Pk "$BACKUP_DIR" | awk 'NR==2 {print $4+0}')"
AVAIL_KB="${AVAIL_KB:-0}"
if [ "$AVAIL_KB" -lt "$NEED_KB" ]; then
  die "磁盘空间不足：需要约 $((NEED_KB/1024))MB，可用 $((AVAIL_KB/1024))MB"
fi

# ---------- 1. 数据库一致性快照 ----------
SNAP="$BACKUP_DIR/qdu-$STAMP.db"
if [ -e "$SNAP" ]; then die "快照文件已存在: $SNAP（同一秒内重复执行？）"; fi

# busy_timeout: 若应用正持锁写入，等待最多 60 秒而不是立即失败
# （stdout 丢弃：PRAGMA busy_timeout 会把设定值打印出来，干扰日志）
if ! sqlite3 "$DB" "PRAGMA busy_timeout=60000; VACUUM INTO '$SNAP';" >/dev/null 2>/tmp/qw-backup-err; then
  ERRMSG="$(head -3 /tmp/qw-backup-err 2>/dev/null || true)"
  rm -f "$SNAP"
  die "数据库快照失败: $ERRMSG"
fi
SRC_SIZE="$(stat -c%s "$DB")"
SNAP_SIZE="$(stat -c%s "$SNAP")"
say "  快照已生成: qdu-$STAMP.db（$((SNAP_SIZE/1024)) KB）"
if [ "$SRC_SIZE" -lt "$SNAP_SIZE" ]; then
  say "  说明: 源库主文件只有 $((SRC_SIZE/1024)) KB，快照有 $((SNAP_SIZE/1024)) KB"
  say "        —— 正常：源库数据大部分还在 -wal 文件里，VACUUM INTO 已完整包含"
  say "        ⚠ 切记：直接拷源库那个 KB 级小文件 = 备份到空库，只能用本快照"
fi

# ---------- 2. 验证快照（关键：防止备份出空库）----------
IC="$(sqlite3 "$SNAP" 'PRAGMA integrity_check;' 2>&1 | head -1)"
if [ "$IC" != "ok" ]; then
  rm -f "$SNAP"
  die "快照完整性检查失败: $IC"
fi

SNAP_TABLES="$(sqlite3 "$SNAP" "SELECT COUNT(*) FROM sqlite_master WHERE type='table';" 2>/dev/null || echo 0)"
SNAP_TABLES="${SNAP_TABLES:-0}"
if [ "$SNAP_TABLES" -lt 5 ]; then
  rm -f "$SNAP"
  die "快照表数量异常（$SNAP_TABLES 张），疑似空库，已删除该快照"
fi

# 关键表行数比对（源库 vs 快照），不一致立即失败
MISMATCH=0
for t in users posts forums comments; do
  s="$(sqlite3 "$DB"   "SELECT COUNT(*) FROM $t;" 2>/dev/null || true)"
  b="$(sqlite3 "$SNAP" "SELECT COUNT(*) FROM $t;" 2>/dev/null || true)"
  if [ -n "$s" ] && [ -n "$b" ]; then
    if [ "$s" != "$b" ]; then
      say "  ✗ 表 $t 行数不一致：源库=$s 快照=$b"
      MISMATCH=1
    else
      say "  ✓ $t: $s 行（源库与快照一致）"
    fi
  fi
done
if [ "$MISMATCH" != "0" ]; then
  rm -f "$SNAP"
  die "快照与源库数据不一致，已删除该快照"
fi
say "  快照验证通过（integrity=ok，$SNAP_TABLES 张表，关键表行数一致）"

# ---------- 3. 打包 uploads/ ----------
UP_FILE=""
if [ -d "$RUN/uploads" ] && [ -n "$(ls -A "$RUN/uploads" 2>/dev/null || true)" ]; then
  if command -v zip >/dev/null 2>&1; then
    UP_FILE="$BACKUP_DIR/uploads-$STAMP.zip"
    ( cd "$RUN" && zip -qr "$UP_FILE" uploads )
  else
    UP_FILE="$BACKUP_DIR/uploads-$STAMP.tar.gz"
    tar -czf "$UP_FILE" -C "$RUN" uploads
  fi
  say "  上传目录已打包: $(basename "$UP_FILE")（$(( $(stat -c%s "$UP_FILE")/1024 )) KB）"
else
  say "  跳过 uploads/（目录不存在或为空）"
fi

# ---------- 4. 轮转：只保留最近 KEEP 份 ----------
# 只识别时间戳格式的批次名（YYYY-MM-DD-HHMMSS），
# 避免误删手工备份文件（如 qdu-auth.db / qdu-auth.db-wal）
BATCHES=()
while IFS= read -r line; do
  [ -n "$line" ] && BATCHES+=("$line")
done < <(
  cd "$BACKUP_DIR" && ls -1 2>/dev/null \
    | sed -nE 's/^qdu-([0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]{6})\.db$/\1/p
               s/^uploads-([0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]{6})\.(zip|tar\.gz)$/\1/p' \
    | sort -u
)
COUNT="${#BATCHES[@]}"
PRUNED=0
if [ "$COUNT" -gt "$KEEP" ]; then
  DEL=$((COUNT-KEEP))
  for ((i=0; i<DEL; i++)); do
    old="${BATCHES[$i]}"
    rm -f "$BACKUP_DIR/qdu-$old.db" "$BACKUP_DIR/uploads-$old.zip" "$BACKUP_DIR/uploads-$old.tar.gz"
    say "  已清理旧备份: $old"
    PRUNED=$((PRUNED+1))
  done
fi
say "  备份轮转: 现有 $COUNT 份，保留上限 $KEEP，本次清理 $PRUNED 份"

# ---------- 5. 汇总 ----------
ls -lt "$BACKUP_DIR" 2>/dev/null | head -6 | sed 's/^/  /'
say "===== 备份完成 ✓ ====="
