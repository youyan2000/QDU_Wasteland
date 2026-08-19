#!/usr/bin/env bash
# backup.sh — 数据自动备份（H5）· 生产服务器定时调用
# 用法：
#   1. 服务器上设置环境变量：QW_ADMIN_EMAIL / QW_ADMIN_PASSWORD（备份专用管理员账号）
#   2. 手动：QW_ADMIN_EMAIL=x QW_ADMIN_PASSWORD=y bash backup.sh
#   3. 每日自动（cron）：crontab -e 加一行
#        0 3 * * * QW_ADMIN_EMAIL=x QW_ADMIN_PASSWORD=y /opt/qdu-wasteland/deploy/backup.sh >> /var/log/qdu-backup.log 2>&1
set -euo pipefail
BASE="${QW_BASE_URL:-http://127.0.0.1:3000}"
EMAIL="${QW_ADMIN_EMAIL:?请设置 QW_ADMIN_EMAIL}"
PASS="${QW_ADMIN_PASSWORD:?请设置 QW_ADMIN_PASSWORD}"
COOKIE="$(mktemp)"
trap 'rm -f "$COOKIE"' EXIT
# 登录获取会话（验证码在部署环境可保持 DISABLE_CAPTCHA=0 时需人工；建议生产启用验证码但备份走管理员豁免）
curl -sf -c "$COOKIE" -X POST "$BASE/api/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"captcha\":\"\",\"captchaId\":\"\"}" >/dev/null
# 触发备份（服务端完成一致性快照 + uploads 打包 + 轮转）
curl -sf -b "$COOKIE" -X POST "$BASE/api/admin/backup"
