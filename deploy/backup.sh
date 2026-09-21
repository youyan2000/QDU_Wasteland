#!/usr/bin/env bash
# backup.sh — 数据自动备份（H5）· 生产服务器定时调用
#
# 用法：
#   1) 手动测试：bash /root/qdu-wasteland/deploy/backup.sh
#   2) 每日自动（crontab 里不用写任何密码，脚本自动读取服务端配置文件）：
#        crontab -e  加一行：
#        0 3 * * * /root/qdu-wasteland/deploy/backup.sh >> /var/log/qdu-backup.log 2>&1
#
# 原理：调用服务端 POST /api/admin/backup →
#   · VACUUM INTO 生成一致性快照（WAL 安全：即使数据都还在 -wal 文件里也完整）
#   · 打包 uploads/ 上传目录为 zip
#   · 轮转：只保留最近 14 份（服务端自动清理）
set -euo pipefail

BASE="${QW_BASE_URL:-http://127.0.0.1:3000}"
ENV_FILE="${QW_ENV_FILE:-/root/qdu-wasteland.env}"
BACKUP_DIR="${QW_BACKUP_DIR:-/root/qdu-wasteland/backups}"

# 凭据：优先 QW_* 环境变量；否则从服务端 env 文件读取（避免把密码写进 crontab）
if [ -z "${QW_ADMIN_EMAIL:-}" ] || [ -z "${QW_ADMIN_PASSWORD:-}" ]; then
  if [ -f "$ENV_FILE" ]; then
    set -a
    # shellcheck disable=SC1090
    . "$ENV_FILE"
    set +a
    QW_ADMIN_EMAIL="${QW_ADMIN_EMAIL:-${ADMIN_EMAIL:-}}"
    QW_ADMIN_PASSWORD="${QW_ADMIN_PASSWORD:-${ADMIN_PASSWORD:-}}"
  fi
fi
EMAIL="${QW_ADMIN_EMAIL:?未找到管理员邮箱：请设置 QW_ADMIN_EMAIL，或确认 $ENV_FILE 内含 ADMIN_EMAIL}"
PASS="${QW_ADMIN_PASSWORD:?未找到管理员密码：请设置 QW_ADMIN_PASSWORD，或确认 $ENV_FILE 内含 ADMIN_PASSWORD}"

echo "[$(date '+%F %T')] 开始备份 ..."
COOKIE="$(mktemp)"
trap 'rm -f "$COOKIE"' EXIT

# 1) 登录（管理员邮箱已豁免验证码）
if ! curl -sf -c "$COOKIE" -X POST "$BASE/api/login" \
  -H 'Content-Type: application/json' \
  -d "$(printf '{"email":"%s","password":"%s","captcha":"","captchaId":""}' "$EMAIL" "$PASS")" >/dev/null; then
  echo "✗ 登录失败：请检查 $ENV_FILE 里的 ADMIN_EMAIL/ADMIN_PASSWORD，以及服务是否可达（$BASE）"
  exit 1
fi

# 2) 触发备份
if ! RESP="$(curl -sf -b "$COOKIE" -X POST "$BASE/api/admin/backup")"; then
  echo "✗ 备份接口调用失败（HTTP 错误）"
  exit 1
fi
echo "  接口返回: $RESP"

# 3) 展示结果（便于 cron 日志核对）
echo "  备份目录（最近 6 项）:"
ls -lt "$BACKUP_DIR" 2>/dev/null | head -7 || echo "  (备份目录不存在)"
echo "[$(date '+%F %T')] ✓ 备份完成"
