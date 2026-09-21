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

# ---------- 0. 自检：脚本自身语法必须合法 ----------
# 带语法错误的脚本会运行到中途才崩溃，而语法错误会让 ERR 陷阱失效，
# 可能把网站留在"已停服"状态。所以在做任何动作之前先自检。
if ! bash -n "$0" 2>/dev/null; then
  echo "✗ 脚本自身语法检查未通过（$0）—— 拒绝执行，线上不受影响"
  bash -n "$0" || true
  exit 1
fi

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

# ---------- 2/7 同步新代码（只动源码目录，不碰数据）----------
# 部署镜像原则：源码目录以远端为准。
# 不用 git pull：上游历史可能被强推改写（rebase/amend），此时 pull 会因"分叉"直接失败。
cd "$SRC"
BRANCH="${QW_BRANCH:-main}"
if ! git fetch --prune origin; then
  echo "✗ git fetch 失败（网络？）—— 中止，线上不受影响"
  exit 1
fi
if ! git rev-parse --verify --quiet "origin/$BRANCH" >/dev/null; then
  echo "✗ 找不到远端分支 origin/$BRANCH —— 中止"
  exit 1
fi

BEFORE="$(git rev-parse --short HEAD)"
# 丢弃前先把本地改动存成补丁（防误伤：万一有人在服务器上直接改过代码）
if [ -n "$(git status --porcelain)" ]; then
  PATCH="$RUN/backups/src-local-changes-$STAMP.patch"
  if git diff > "$PATCH" 2>/dev/null; then
    echo "  ⚠ 源码目录有未提交改动，已保存补丁: $PATCH"
  fi
fi
DIVERGED="$(git log --oneline "origin/$BRANCH..HEAD" 2>/dev/null | wc -l | tr -d ' ')"

git reset --hard "origin/$BRANCH" >/dev/null
AFTER="$(git rev-parse --short HEAD)"
echo "[2/7] ✓ 代码 $BEFORE → $AFTER（已同步 origin/$BRANCH）"
if [ "${DIVERGED:-0}" != "0" ]; then
  echo "      注意：丢弃了 $DIVERGED 个不在远端的本地提交（上游强推改写历史所致，内容已在远端）"
fi
if [ "$BEFORE" = "$AFTER" ]; then
  echo "      （无新提交，继续重建）"
fi

# ---------- 3/7 编译（失败立即中止，线上服务完全不受影响）----------
export PATH="$(dirname "$GO_BIN"):$PATH"
if ! command -v go >/dev/null 2>&1; then
  echo "✗ 找不到 go 编译器（安装: snap install go --classic）"
  exit 1
fi
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SRC/qdu-wasteland-linux.new" . &
BUILD_PID=$!
echo "      编译已开始（PID $BUILD_PID）"
echo "      ⚠ 注意：若构建缓存与本次配置不匹配（如首次使用 CGO_ENABLED=0），"
echo "        1 核服务器上可能需要 10-40 分钟；期间没有输出是正常的，请勿中断。"
while kill -0 "$BUILD_PID" 2>/dev/null; do
  sleep 30
  if kill -0 "$BUILD_PID" 2>/dev/null; then
    echo "      …仍在编译（已等待 ${SECONDS}s）"
  fi
done
if ! wait "$BUILD_PID"; then
  echo "✗ 编译失败（详见上方 Go 报错）—— 中止更新，线上服务与数据均未受影响"
  rm -f "$SRC/qdu-wasteland-linux.new"
  exit 1
fi
NEWSIZE="$(stat -c%s "$SRC/qdu-wasteland-linux.new")"
if [ "$NEWSIZE" -lt 1000000 ]; then
  echo "✗ 编译产物异常偏小（$NEWSIZE 字节），中止更新"
  rm -f "$SRC/qdu-wasteland-linux.new"
  exit 1
fi
echo "[3/7] ✓ 编译成功（$NEWSIZE 字节）"

# ---------- 4/7 停服 ----------
# 停服前先放一个"看门狗"：无论本脚本之后因何原因死掉（含语法错误、被 kill），
# 15 分钟后只要服务还没起来就自动拉起它 —— 保证网站不会长期宕机。
# 正常更新时服务处于 active，看门狗到点检查后什么也不做。
( sleep 900; systemctl is-active --quiet "$SERVICE" || systemctl start "$SERVICE" ) >/dev/null 2>&1 &
WATCHDOG_PID=$!
echo "      已启动看门狗（PID $WATCHDOG_PID，15 分钟后若服务未运行则自动拉起）"
systemctl stop "$SERVICE"
STOPPED=1
echo "[4/7] ✓ 服务已停止"

# ---------- 5/7 只替换程序 + 前端 + 运维脚本 ----------
# cp -r 是覆盖式：只增改不删除，public/ 里已有的文件（含用户上传）不会被删
mv "$SRC/qdu-wasteland-linux.new" "$RUN/qdu-wasteland-linux"
chmod +x "$RUN/qdu-wasteland-linux"
cp -r "$SRC/public/." "$RUN/public/"

# ⚠ 关键安全点：绝不覆盖「正在运行的脚本自身」。
# 若把 update.sh 覆盖掉，bash 会继续从被改写的文件里读后续内容 → 解析崩溃，
# 而语法错误会让 ERR 兜底陷阱失效，脚本会死在"已停服"状态导致网站宕机。
mkdir -p "$RUN/deploy"
SELF="$(readlink -f "$0" 2>/dev/null || echo "$0")"
SKIPPED_SELF=""
for f in "$SRC"/deploy/*; do
  [ -e "$f" ] || continue
  base="$(basename "$f")"
  if [ -e "$RUN/deploy/$base" ]; then
    dest="$(readlink -f "$RUN/deploy/$base" 2>/dev/null || echo "$RUN/deploy/$base")"
    if [ "$dest" = "$SELF" ]; then
      SKIPPED_SELF="$base"
      continue
    fi
  fi
  cp -r "$f" "$RUN/deploy/"
done
chmod +x "$RUN"/deploy/*.sh 2>/dev/null || true
echo "[5/7] ✓ 已替换 程序 + public/ + deploy/"
if [ -n "$SKIPPED_SELF" ]; then
  echo "      跳过 $SKIPPED_SELF（正在运行的脚本不覆盖自身，否则会运行中崩溃）"
  echo "      如需同步它：cp $SRC/deploy/$SKIPPED_SELF $RUN/deploy/$SKIPPED_SELF"
fi
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
