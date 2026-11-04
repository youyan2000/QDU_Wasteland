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

# ---------- 3. 环境文件 ----------
install -m 0640 -o root -g qdu "$ENV_OLD" "$ENV_NEW"
say "环境文件已安装: $ENV_NEW"

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
