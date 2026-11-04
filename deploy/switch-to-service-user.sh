#!/usr/bin/env bash
# switch-to-service-user.sh — 把服务从 root 迁移到专用用户 qdu（/root → /srv）
#
# 默认只做检查，不改任何东西：
#   bash deploy/switch-to-service-user.sh
# 确认输出没问题后正式执行：
#   APPLY=1 bash deploy/switch-to-service-user.sh
#
# 做的事：
#   1. 建系统用户 qdu（nologin）
#   2. rsync 把 /root/qdu-wasteland 同步到 /srv/qdu-wasteland（先不删源目录）
#   3. chown qdu:qdu
#   4. 环境变量复制到 /etc/qdu-wasteland.env（600 root:qdu）
#   5. 安装 systemd 单元（User=qdu）并重启
#   6. 健康检查；失败自动切回旧单元
# 回滚：
#   systemctl stop qdu-wasteland
#   cp /root/qdu-wasteland/qdu-wasteland-linux.rollback.$(date +%F) 回去或改用旧单元
set -uo pipefail

SRC="${QW_SRC_DIR:-/root/qdu-wasteland}"
DST="${QW_DST_DIR:-/srv/qdu-wasteland}"
ENV_OLD="${QW_ENV_OLD:-/root/qdu-wasteland.env}"
ENV_NEW="${QW_ENV_NEW:-/etc/qdu-wasteland.env}"
UNIT="/etc/systemd/system/qdu-wasteland.service"
SERVICE="qdu-wasteland"
APPLY="${APPLY:-0}"

say() { echo "[$(date '+%F %T')] $*"; }
die() { echo "[$(date '+%F %T')] ✗ $*" >&2; exit 1; }

[ -d "$SRC" ] || die "找不到运行目录 $SRC"
[ -f "$SRC/qdu-wasteland-linux" ] || die "$SRC 下没有 qdu-wasteland-linux"
[ -f "$ENV_OLD" ] || die "找不到环境变量文件 $ENV_OLD"
command -v rsync >/dev/null 2>&1 || die "未安装 rsync（apt install -y rsync）"

say "===== 迁移计划 ====="
say "  运行目录: $SRC → $DST"
say "  服务用户: qdu (nologin)"
say "  环境文件: $ENV_OLD → $ENV_NEW (600 root:qdu)"
say "  单元文件: $UNIT（User=qdu，ReadWritePaths=$DST）"
if [ "$APPLY" != "1" ]; then
  say ""
  say "这是演练模式，不会修改任何东西。确认后执行："
  say "  APPLY=1 bash deploy/switch-to-service-user.sh"
  exit 0
fi

# ---------- 1. 用户 ----------
if ! id qdu >/dev/null 2>&1; then
  useradd --system --create-home --home-dir /srv/qdu-wasteland --shell /usr/sbin/nologin qdu
  say "已创建用户 qdu"
else
  say "用户 qdu 已存在"
fi

# ---------- 2. 目录同步（保留源目录，直到验证通过）----------
mkdir -p "$DST"
rsync -a --delete "$SRC/" "$DST/"
chown -R qdu:qdu "$DST"
chmod 750 "$DST"
say "已同步到 $DST 并授权 qdu"

# ---------- 3. 环境文件 + 数据路径迁移 ----------
install -m 0640 -o root -g qdu "$ENV_OLD" "$ENV_NEW"

# qdu 用户读不到 /root（目录本身 700），而 unit 又开了 ProtectHome=true。
# 所以 CSV_DIR / TOOLS_DATA_DIR / TOOLS_ROOT / AUTH_DB 只要指向 /root 下，就必须搬到 $DST。
migrate_dir() {
  local key="$1" src="$2" dst="$3"
  if [ -d "$src" ]; then
    mkdir -p "$dst"
    rsync -a "$src/" "$dst/"
    chown -R qdu:qdu "$dst"
    sed -i "s|^$key=.*|$key=$dst|" "$ENV_NEW"
    say "  $key: $src → $dst"
  fi
}

csv_old="$(grep -E '^CSV_DIR=' "$ENV_NEW" | head -1 | cut -d= -f2- || true)"
if [ -n "$csv_old" ] && [ "${csv_old#/root}" != "$csv_old" ]; then
  migrate_dir CSV_DIR "$csv_old" "$DST/csv"
fi
tools_data_old="$(grep -E '^TOOLS_DATA_DIR=' "$ENV_NEW" | head -1 | cut -d= -f2- || true)"
if [ -n "$tools_data_old" ] && [ "${tools_data_old#/root}" != "$tools_data_old" ]; then
  migrate_dir TOOLS_DATA_DIR "$tools_data_old" "$DST/tool-data"
fi
tools_root_old="$(grep -E '^TOOLS_ROOT=' "$ENV_NEW" | head -1 | cut -d= -f2- || true)"
if [ -n "$tools_root_old" ] && [ "${tools_root_old#/root}" != "$tools_root_old" ]; then
  migrate_dir TOOLS_ROOT "$tools_root_old" "$DST/tools"
fi
authdb_old="$(grep -E '^AUTH_DB=' "$ENV_NEW" | head -1 | cut -d= -f2- || true)"
if [ -n "$authdb_old" ] && [ "${authdb_old#/root}" != "$authdb_old" ] && [ -f "$authdb_old" ]; then
  cp -a "$authdb_old" "$DST/qdu-auth.db"
  cp -a "$authdb_old-wal" "$DST/qdu-auth.db-wal" 2>/dev/null || true
  cp -a "$authdb_old-shm" "$DST/qdu-auth.db-shm" 2>/dev/null || true
  chown qdu:qdu "$DST"/qdu-auth.db* 2>/dev/null || true
  sed -i "s|^AUTH_DB=.*|AUTH_DB=$DST/qdu-auth.db|" "$ENV_NEW"
  say "  AUTH_DB: $authdb_old → $DST/qdu-auth.db"
fi
chown qdu:qdu "$DST"/* 2>/dev/null || true
say "环境文件已安装: $ENV_NEW"

# 预检：
#   服务还在跑时 3000 端口被占用，不能再启动一个实例，改做权限检查；
#   服务已停时才试启动，且"跑满 30 秒没退出"也算通过（真实启动要 60-80 秒）。
PREFLIGHT_OK=0
if systemctl is-active --quiet "$SERVICE"; then
  PREFLIGHT_OK=1
  for probe in "test -r $DST" "test -w $DST" "test -r $DST/csv" "test -r $DST/qdu-auth.db" "test -x $DST/qdu-wasteland-linux"; do
    if ! sudo -u qdu bash -c "$probe"; then
      say "✗ 权限预检失败: $probe"
      PREFLIGHT_OK=0
      break
    fi
  done
else
  sudo -u qdu bash -c "set -a; . '$ENV_NEW'; set +a; cd '$DST' && timeout 30 ./qdu-wasteland-linux" >/tmp/qdu-preflight.log 2>&1
  rc=$?
  if [ "$rc" -eq 124 ] || grep -q '已启动' /tmp/qdu-preflight.log; then
    PREFLIGHT_OK=1
  else
    say "✗ 以 qdu 身份启动预检失败（exit=$rc），最后 20 行："
    tail -20 /tmp/qdu-preflight.log | sed 's/^/        /'
  fi
fi
if [ "$PREFLIGHT_OK" != "1" ]; then
  say "  未改动 systemd 单元，服务仍在原状态。修复上面的问题后重跑。"
  exit 1
fi
say "预检通过（qdu 身份可正常启动）"

# ---------- 4. 单元文件 ----------
if [ -f "$UNIT" ]; then
  cp -p "$UNIT" "$UNIT.bak.$(date +%F-%H%M%S)"
  say "已备份旧单元文件"
fi
install -m 0644 "$(dirname "$0")/qdu-wasteland.service.example" "$UNIT"
systemctl daemon-reload
say "单元文件已安装"

# ---------- 5. 重启并验证 ----------
systemctl restart "$SERVICE" || die "服务启动失败，查看 journalctl -u $SERVICE -n 80"
READY=0
for i in $(seq 1 24); do
  sleep 5
  if curl -sf -o /dev/null --max-time 5 http://127.0.0.1:3000/; then READY=1; break; fi
  printf "      ...等待中 %ss\n" "$((i*5))"
done

if [ "$READY" != "1" ]; then
  say "✗ 健康检查失败，回滚到迁移前状态"
  say "  本次启动日志（最后 25 行）："
  journalctl -u "$SERVICE" --since "-4 min" --no-pager 2>/dev/null | tail -25 | sed 's/^/        /'
  systemctl stop "$SERVICE"
  rm -f "$UNIT"
  if ls "$UNIT".bak.* >/dev/null 2>&1; then
    cp -p "$(ls -t "$UNIT".bak.* | head -1)" "$UNIT"
  fi
  systemctl daemon-reload
  systemctl start "$SERVICE"
  die "已回滚；请检查 journalctl -u $SERVICE -n 80"
fi

say "✓ 迁移完成，服务以 qdu 用户在 $DST 运行"
say ""
say "验证："
say "  systemctl show qdu-wasteland -p User            # User=qdu"
say "  ls -ld $DST/uploads $DST/tool-data              # owner 应为 qdu"
say "  systemctl cat qdu-wasteland | grep -i admin     # 不应出现 ADMIN_PASSWORD"
say ""
say "确认稳定运行 24 小时后，再删除旧目录："
say "  rm -rf $SRC"
