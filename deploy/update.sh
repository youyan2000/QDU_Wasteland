#!/usr/bin/env bash
# update.sh — QDU Wasteland 安全更新脚本
# 在服务器上执行：bash /root/qdu-wasteland/deploy/update.sh
#
# 设计原则：只替换「程序 + 前端」，绝不触碰任何数据
#   ✅ 会覆盖：qdu-wasteland-linux（程序）、public/（网页）、deploy/（运维脚本，先备份）
#   ❌ 永不触碰：qdu-auth.db(+wal/shm) 数据库、uploads/ 用户上传、.env 配置
# 编译失败 → 立即中止（线上不受影响）
# 启动后健康检查失败 → 自动回滚旧程序并重启
# 中途任何异常 → 自动恢复旧程序并拉起服务（绝不让网站停在宕机状态）
set -euo pipefail

SRC="${QW_SRC_DIR:-/root/qdu-wasteland-src}"      # 源码目录（git clone）
RUN="${QW_RUN_DIR:-/root/qdu-wasteland}"          # 运行目录（程序 + 数据）
SERVICE="${QW_SERVICE:-qdu-wasteland}"            # systemd 服务名
GO_BIN="${QW_GO_BIN:-/snap/bin/go}"               # Go 编译器
HEALTH_URL="${QW_HEALTH_URL:-http://127.0.0.1:3000}"
STAMP="$(date +%Y%m%d-%H%M%S)"
STOPPED=0

# ---------- 异常兜底：绝不让网站停在宕机状态 ----------
on_error() {
  rc=$?
  echo ""
  echo "✗ 更新过程中出错（退出码 $rc）"
  if [ "$STOPPED" = "1" ]; then
    echo "  服务当前处于停止状态 —— 正在恢复旧程序并重新启动..."
    if [ -f "$RUN/backups/qdu-wasteland-linux.$STAMP" ]; then
      cp -p "$RUN/backups/qdu-wasteland-linux.$STAMP" "$RUN/qdu-wasteland-linux"
      echo "  已还原更新前的程序"
    fi
    systemctl start "$SERVICE" || true
    echo "  已尝试拉起服务，请确认: systemctl status $SERVICE --no-pager"
  else
    echo "  线上程序与数据均未被改动，网站不受影响。"
  fi
  exit "$rc"
}
trap on_error ERR

echo "======================================================"
echo " QDU Wasteland 安全更新  $STAMP"
echo "  源码: $SRC"
echo "  运行: $RUN"
echo "======================================================"

if [ ! -d "$SRC/.git" ]; then echo "✗ 找不到源码目录 $SRC（需要 git 仓库）"; exit 1; fi
if [ ! -d "$RUN" ]; then echo "✗ 找不到运行目录 $RUN"; exit 1; fi

# ---------- 1/7 备份数据（数据库 + 旧程序 + 旧运维脚本）----------
mkdir -p "$RUN/backups"
for f in qdu-auth.db qdu-auth.db-wal qdu-auth.db-shm; do
  if [ -f "$RUN/$f" ]; then
    cp -p "$RUN/$f" "$RUN/backups/$f.$STAMP"
    echo "  已备份 $f"
  fi
done
if [ -f "$RUN/qdu-wasteland-linux" ]; then
  cp -p "$RUN/qdu-wasteland-linux" "$RUN/backups/qdu-wasteland-linux.$STAMP"
  echo "  已备份 qdu-wasteland-linux（旧程序）"
fi
if [ -d "$RUN/deploy" ]; then
  cp -rp "$RUN/deploy" "$RUN/backups/deploy.$STAMP"
  echo "  已备份 deploy/（旧运维脚本）"
fi
echo "[1/7] ✓ 备份完成 → $RUN/backups/ (*.$STAMP)"

# ---------- 2/7 拉取新代码（只动源码目录，不碰数据）----------
cd "$SRC"
BEFORE="$(git rev-parse --short HEAD)"
git pull --ff-only
AFTER="$(git rev-parse --short HEAD)"
echo "[2/7] ✓ 代码 $BEFORE → $AFTER"
if [ "$BEFORE" = "$AFTER" ]; then
  echo "      （无新提交，继续重建）"
fi

# ---------- 3/7 编译（失败立即中止，线上服务完全不受影响）----------
export PATH="$(dirname "$GO_BIN"):$PATH"
if ! command -v go >/dev/null 2>&1; then
  echo "✗ 找不到 go 编译器（安装: snap install go --classic）"
  exit 1
fi
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SRC/qdu-wasteland-linux.new" .
NEWSIZE="$(stat -c%s "$SRC/qdu-wasteland-linux.new")"
if [ "$NEWSIZE" -lt 1000000 ]; then
  echo "✗ 编译产物异常偏小（$NEWSIZE 字节），中止更新"
  rm -f "$SRC/qdu-wasteland-linux.new"
  exit 1
fi
echo "[3/7] ✓ 编译成功（$NEWSIZE 字节）"

# ---------- 4/7 停服 ----------
systemctl stop "$SERVICE"
STOPPED=1
echo "[4/7] ✓ 服务已停止"

# ---------- 5/7 只替换程序 + 前端 + 运维脚本 ----------
# cp -r 是覆盖式：只增改不删除，public/ 里已有的文件（含用户上传）不会被删
mv "$SRC/qdu-wasteland-linux.new" "$RUN/qdu-wasteland-linux"
chmod +x "$RUN/qdu-wasteland-linux"
cp -r "$SRC/public/." "$RUN/public/"
mkdir -p "$RUN/deploy"
cp -r "$SRC/deploy/." "$RUN/deploy/"
chmod +x "$RUN"/deploy/*.sh 2>/dev/null || true
echo "[5/7] ✓ 已替换 程序 + public/ + deploy/"
echo "      （数据库 qdu-auth.db / uploads/ / .env 全程未改动）"

# ---------- 6/7 启动（冷启动需 3-4 分钟：加载 4457 门课 / 38123 条开课）----------
systemctl start "$SERVICE"
echo "[6/7] ✓ 服务已启动，等待就绪（最长 6 分钟，期间报 502 属正常）..."

# ---------- 7/7 健康检查，失败自动回滚 ----------
READY=0
for i in $(seq 1 36); do
  sleep 10
  if curl -sf -o /dev/null --max-time 5 "$HEALTH_URL"; then
    READY=1
    break
  fi
  printf "      ...等待中 %ss\n" "$((i*10))"
done

if [ "$READY" != "1" ]; then
  echo "[7/7] ✗ 健康检查失败 —— 自动回滚到更新前的程序"
  systemctl stop "$SERVICE"
  cp -p "$RUN/backups/qdu-wasteland-linux.$STAMP" "$RUN/qdu-wasteland-linux"
  systemctl start "$SERVICE"
  echo "      已回滚。数据从未被改动，网站会恢复成更新前的样子。"
  echo "      排查日志: journalctl -u $SERVICE -n 100 --no-pager"
  trap - ERR
  exit 1
fi

trap - ERR
echo "[7/7] ✓ 网站正常响应 $HEALTH_URL"
echo "======================================================"
echo " 更新完成 ✓  版本 $BEFORE → $AFTER"
echo " 数据状态：数据库 / uploads / .env 全程未被改动"
echo " 如需回滚："
echo "   systemctl stop $SERVICE"
echo "   cp $RUN/backups/qdu-wasteland-linux.$STAMP $RUN/qdu-wasteland-linux"
echo "   systemctl start $SERVICE"
echo "======================================================"
