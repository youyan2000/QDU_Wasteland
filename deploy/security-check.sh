#!/usr/bin/env bash
# security-check.sh — 上线安全检查（只读，不改任何东西）
#
# 用法：
#   bash /root/qdu-wasteland/deploy/security-check.sh
#
# 输出 PASS / WARN / FAIL 三类；有 FAIL 时退出码为 1。
# 每一项都附了修复命令，照着做即可。
set -uo pipefail

RUN="${QW_RUN_DIR:-/root/qdu-wasteland}"
SERVICE="${QW_SERVICE:-qdu-wasteland}"
FAILED=0
WARNED=0

ok()   { echo "  PASS  $*"; }
warn() { echo "  WARN  $*"; WARNED=$((WARNED+1)); }
bad()  { echo "  FAIL  $*"; FAILED=$((FAILED+1)); }

echo "===== 上线安全检查 $(date '+%F %T') ====="
echo

echo "[1/7] ClamAV 病毒扫描"
if command -v clamscan >/dev/null 2>&1; then
  ok "已安装 clamscan: $(command -v clamscan)"
else
  warn "未安装 clamscan（上传文件不会被病毒扫描）"
  echo "        修复: apt install -y clamav && freshclam"
fi
CLAM_ENV=""
for f in /etc/qdu-wasteland.env "$RUN/.env" /root/qdu-wasteland.env; do
  [ -f "$f" ] || continue
  if grep -q '^CLAMAV_CMD=' "$f" 2>/dev/null; then
    CLAM_ENV="$f:$(grep '^CLAMAV_CMD=' "$f")"
  fi
done
if [ -n "$CLAM_ENV" ]; then
  ok "已配置 $CLAM_ENV"
else
  warn "环境文件里没有 CLAMAV_CMD（程序会跳过扫描）"
  echo "        修复: 在 EnvironmentFile 里加 CLAMAV_CMD=/usr/bin/clamscan 后重启服务"
fi
echo

echo "[2/7] systemd 是否以专用用户运行"
UNIT_USER="$(systemctl show "$SERVICE" -p User --value 2>/dev/null || true)"
if [ -z "$UNIT_USER" ] || [ "$UNIT_USER" = "root" ]; then
  bad "服务以 root 运行（$SERVICE: User='${UNIT_USER:-<empty>}'）"
  echo "        修复: APPLY=1 bash $RUN/deploy/switch-to-service-user.sh"
else
  ok "以专用用户运行: $UNIT_USER"
fi
# 只看真正的 Environment= 行；注释里提到变量名不算泄露（之前的版本会把这个当 FAIL）。
UNIT_TEXT="$(systemctl cat "$SERVICE" 2>/dev/null || true)"
if printf '%s\n' "$UNIT_TEXT" \
   | grep -E '^[[:space:]]*Environment=' \
   | grep -qE 'ADMIN_PASSWORD|MAIL_PASS|AI_API_KEY|SMTP_PASS'; then
  bad "unit 文件里出现明文凭据（systemctl cat 就能看到）"
  echo "        修复: 把凭据移到 EnvironmentFile（600 权限），unit 里只留 EnvironmentFile="
else
  ok "unit 文件里没有明文凭据（EnvironmentFile 里的不算）"
fi
echo

echo "[3/7] 备份加密"
if [ -f "${QW_BACKUP_KEY_FILE:-$RUN/backup.key}" ]; then
  PERM="$(stat -c '%a' "${QW_BACKUP_KEY_FILE:-$RUN/backup.key}" 2>/dev/null || echo '?')"
  if [ "$PERM" = "600" ]; then ok "备份密钥存在且权限 600"; else warn "备份密钥权限是 $PERM（应为 600）"; fi
else
  warn "还没有备份密钥（下次 backup.sh 会生成）"
fi
ENC_COUNT="$(find "$RUN/backups" -maxdepth 1 -name '*.enc' 2>/dev/null | wc -l | tr -d ' ')"
PLAIN_COUNT="$(find "$RUN/backups" -maxdepth 1 \( -name '*.db' -o -name '*.zip' -o -name '*.tar.gz' \) 2>/dev/null | wc -l | tr -d ' ')"
if [ "$ENC_COUNT" -gt 0 ] && [ "$PLAIN_COUNT" -eq 0 ]; then
  ok "备份已全部加密（$ENC_COUNT 个 .enc，无明文）"
elif [ "$PLAIN_COUNT" -gt 0 ]; then
  bad "存在 $PLAIN_COUNT 个明文备份（含全部用户数据）"
  echo "        修复: 重新跑一次 bash $RUN/deploy/backup.sh（新备份会加密并删除明文）"
  echo "              再手工加密历史明文: 见 docs/SECURITY-ACTIONS-2026-11-05.md"
else
  warn "还没有任何备份产物"
fi
echo

echo "[4/7] 文件下载是否需要登录"
API="http://127.0.0.1:3000"
CODE="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$API/api/files?code=x" 2>/dev/null || echo 000)"
if [ "$CODE" = "401" ]; then
  ok "匿名访问 /api/files 返回 401"
elif [ "$CODE" = "000" ]; then
  warn "本机 3000 端口无响应，跳过（部署后再跑）"
else
  bad "匿名访问 /api/files 返回 $CODE（应为 401）"
  echo "        修复: 部署最新代码（文件列表/下载/预览已要求登录）"
fi
DL="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$API/api/files/1/download" 2>/dev/null || echo 000)"
if [ "$DL" = "401" ]; then ok "匿名下载 /api/files/1/download 返回 401"; else warn "匿名下载返回 $DL（期望 401）"; fi
echo

echo "[5/7] nginx 版本号"
if command -v nginx >/dev/null 2>&1; then
  # 只认真正生效的指令行；`# server_tokens off;` 这种注释不算。
  # 注意先取输出再 grep：pipefail 下 grep -q 提前退出会让 nginx 收到 SIGPIPE，误判为失败。
  NGINX_CONF="$(nginx -T 2>/dev/null || true)"
  if printf '%s\n' "$NGINX_CONF" | grep -qE '^[[:space:]]*server_tokens[[:space:]]+off;'; then
    ok "已配置 server_tokens off"
  else
    bad "未关闭 server_tokens（响应头会暴露 nginx 版本）"
    echo "        修复: 在 nginx.conf 的 http{} 里加 server_tokens off; 然后 nginx -t && systemctl reload nginx"
  fi
else
  warn "未安装 nginx（如果走 Tunnel 直连则可忽略）"
fi
echo

echo "[6/7] 监听端口"
ss -tlnp 2>/dev/null | grep -E ':(3000|80|443|8443|22)\s' | sed 's/^/  /' || true
if ss -tln 2>/dev/null | grep -q '127.0.0.1:3000'; then
  ok "应用只监听 127.0.0.1:3000"
else
  bad "应用没有监听 127.0.0.1:3000（检查 PORT 配置）"
fi
if ss -tln 2>/dev/null | grep -qE '0\.0\.0\.0:(80|443)|\[::\]:(80|443)'; then
  # 端口在听不等于对全网开放：检查 ufw 是否只放行了指定网段。
  UFW_TEXT="$(ufw status 2>/dev/null || true)"
  if printf '%s\n' "$UFW_TEXT" | grep -E '^(80|443)/tcp' | grep -qv 'Anywhere'; then
    ok "80/443 仅对指定来源开放（ufw 已限制网段）"
  else
    warn "源站 80/443 对所有来源开放"
    echo "        修复见 docs/SECURITY-ACTIONS-2026-11-05.md 第 6 节"
  fi
else
  ok "源站 80/443 未对公网开放"
fi
echo

echo "[7/7] 上传目录里是否有可疑文件"
SUSPECT="$(find "$RUN/uploads" -maxdepth 2 -type f \( -iname '*.exe' -o -iname '*.sh' -o -iname '*.php' -o -iname '*.jsp' -o -iname '*.doc' -o -iname '*.xls' \) 2>/dev/null | head -20)"
if [ -n "$SUSPECT" ]; then
  warn "uploads/ 里有需要人工确认的文件："
  echo "$SUSPECT" | sed 's/^/        /'
else
  ok "uploads/ 里没有可执行/老式文档文件"
fi
echo

echo "===== 汇总：FAIL=$FAILED  WARN=$WARNED ====="
if [ "$FAILED" -gt 0 ]; then
  echo "有必须处理的问题，按上面的「修复」逐条执行。"
  exit 1
fi
echo "没有 FAIL 项。"
