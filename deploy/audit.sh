#!/usr/bin/env bash
# audit.sh — 本机部署防御性审查（只读，不做网络请求或配置修改）
# 仅检查：本机服务/监听、Nginx 配置语法、反代信任边界和敏感值脱敏。
# 不执行漏洞复现、绕过、UA 轮换、慢请求、压测或任何外网探测。
set -u

RUN="${QW_RUN_DIR:-/root/qdu-wasteland}"
SERVICE="${QW_SERVICE:-qdu-wasteland}"
SVC_FILE="/etc/systemd/system/${SERVICE}.service"
PASS=0
WARN=0
FAIL=0

section() { printf '\n== %s ==\n' "$1"; }
ok() { printf 'PASS: %s\n' "$1"; PASS=$((PASS+1)); }
warn() { printf 'WARN: %s\n' "$1"; WARN=$((WARN+1)); }
fail() { printf 'FAIL: %s\n' "$1"; FAIL=$((FAIL+1)); }
exists() { command -v "$1" >/dev/null 2>&1; }

section '审查范围'
printf '%s\n' '只读本机信息；不访问外网、不发送请求、不读取密钥内容。'
printf '运行目录: %s\n' "$RUN"
printf '服务名: %s\n' "$SERVICE"

section '服务与应用绑定'
if exists systemctl; then
  state="$(systemctl is-active "$SERVICE" 2>/dev/null || true)"
  case "$state" in
    active) ok "应用服务 active" ;;
    *) warn "应用服务状态为 ${state:-unknown}" ;;
  esac
else
  warn 'systemctl 不可用，跳过服务状态'
fi
if [ -f "$SVC_FILE" ]; then
  if grep -Eq '(^|[[:space:]])Environment(File)?=' "$SVC_FILE" 2>/dev/null; then
    ok 'systemd 单元使用环境配置（仅检查键名，不输出值）'
  else
    warn 'systemd 单元未发现 Environment/EnvironmentFile；请确认凭据不写入命令行'
  fi
fi
if [ -f "$RUN/qdu-wasteland-linux" ]; then
  ok '运行目录存在应用文件'
else
  warn '运行目录未发现应用文件'
fi

# Show only environment key names; never print the values themselves.
for env_file in /root/qdu-wasteland.env "$RUN/.env"; do
  if [ -f "$env_file" ]; then
    keys="$(grep -oE '^[[:space:]]*[A-Za-z_][A-Za-z0-9_]*[[:space:]]*=' "$env_file" 2>/dev/null | sed -E 's/[[:space:]=]+$//' | sort -u | tr '\n' ' ' || true)"
    printf '环境文件变量名（值隐藏）: %s\n' "${keys:-未识别}"
  fi
done

section '本机监听（不含进程命令行）'
if exists ss; then
  listeners="$(ss -H -ltn 2>/dev/null || true)"
  if [ -n "$listeners" ]; then
    printf '%s\n' "$listeners" | sed -E 's/[[:space:]]+/ /g' | sed -E 's/^ /  /' | cut -c1-180
  else
    warn '未读取到 TCP 监听'
  fi
  if printf '%s\n' "$listeners" | grep -Eq '(^|[[:space:]])(0\.0\.0\.0:|\*:|\[::\]:)(80|443|3000)([[:space:]]|$)'; then
    warn '发现公网地址监听；Tunnel 模式应仅让 cloudflared/Nginx 使用回环地址'
  fi
  if printf '%s\n' "$listeners" | grep -Eq '127\.0\.0\.1:3000|\[::1\]:3000'; then
    ok '应用端口 3000 绑定回环地址'
  else
    warn '未确认应用端口 3000 仅绑定回环地址'
  fi
else
  warn 'ss 不可用，跳过监听审查'
fi

section 'Nginx 配置语法与静态审查'
if exists nginx; then
  if nginx -t >/dev/null 2>&1; then
    ok 'nginx -t 通过'
  else
    fail 'nginx -t 失败（未输出配置内容）'
  fi
  stream_checked=0
  for dir in /etc/nginx/sites-enabled /etc/nginx/conf.d /etc/nginx/stream.d; do
    [ -d "$dir" ] || continue
    [ "$dir" = /etc/nginx/stream.d ] && stream_checked=1
    if grep -RqsE 'proxy_set_header[[:space:]]+X-Forwarded-For[[:space:]]+\$proxy_add_x_forwarded_for' "$dir" 2>/dev/null; then
      fail "$dir 含有会保留客户端伪造链的 proxy_add_x_forwarded_for"
    fi
    if grep -RqsE 'proxy_set_header[[:space:]]+X-Real-IP[[:space:]]+\$http_' "$dir" 2>/dev/null; then
      fail "$dir 从入站请求头设置 X-Real-IP"
    fi
    if grep -RqsE 'listen[[:space:]]+80([^0-9]|$)' "$dir" 2>/dev/null && ! grep -RqsE 'return[[:space:]]+30[1278][[:space:]]+https://' "$dir" 2>/dev/null; then
      warn "$dir 发现 HTTP listener，请确认只用于固定 canonical HTTPS 重定向或拒绝未知 Host"
    fi
  done
  if [ "$stream_checked" -eq 1 ]; then
    ok '发现 stream.d 并纳入 nginx -t 配置语法检查；不输出其配置内容'
  else
    warn '未发现 /etc/nginx/stream.d；若 stream 配置在其他路径，请用 nginx -t 并人工核对 include'
  fi
  if grep -RqsE 'server_tokens[[:space:]]+off' /etc/nginx 2>/dev/null; then
    ok 'Nginx 隐藏版本号配置存在'
  else
    warn '未确认 server_tokens off'
  fi
else
  warn 'nginx 不可用，跳过 nginx -t 与配置审查'
fi

section 'Tunnel 信任边界（本机静态检查）'
TUNNEL_FILE="${QW_TUNNEL_NGINX_CONF:-$RUN/deploy/nginx-tunnel.conf.example}"
if [ -f "$TUNNEL_FILE" ]; then
  ok '发现 Tunnel -> loopback Nginx 模板'
  if grep -Eq 'set_real_ip_from[[:space:]]+127\.0\.0\.1|set_real_ip_from[[:space:]]+::1' "$TUNNEL_FILE" && \
     grep -Eq 'real_ip_header[[:space:]]+CF-Connecting-IP' "$TUNNEL_FILE"; then
    ok '真实 IP 信任范围限制为 loopback tunnel peer'
  else
    warn 'Tunnel 模板未确认真实 IP loopback 信任边界'
  fi
  if grep -Eq '0\.0\.0\.0/0|proxy_add_x_forwarded_for' "$TUNNEL_FILE"; then
    fail 'Tunnel 模板含任意来源或链式转发信任'
  fi
else
  warn '未发现 Tunnel 模板（可通过 QW_TUNNEL_NGINX_CONF 指定）'
fi

section '防火墙与封禁策略（仅配置摘要）'
if exists ufw; then
  ufw_status="$(ufw status 2>/dev/null | sed -n '1,4p' || true)"
  printf '%s\n' "$ufw_status" | sed -E 's/[[:space:]]+/ /g'
else
  warn 'ufw 不可用；请人工确认云防火墙/主机防火墙'
fi
if exists fail2ban-client; then
  jails="$(fail2ban-client status 2>/dev/null | sed -n '1,3p' || true)"
  printf '%s\n' "$jails" | sed -E 's/[[:space:]]+/ /g'
  if fail2ban-client status 2>/dev/null | grep -qiE 'cloudflare|cdn|tunnel'; then
    warn '检测到可能针对 CDN/Tunnel 的 jail；不要封禁 CDN 出口 IP，应在可信客户端 IP 解析层处理'
  fi
else
  printf '%s\n' 'fail2ban-client 不可用（不代表必须安装）'
fi

section '日志与轮转模板'
for f in /etc/logrotate.d/nginx-qdu-wasteland /etc/systemd/journald.conf.d/qdu-wasteland.conf; do
  if [ -f "$f" ]; then ok "发现日志策略配置: $(basename "$f")"; else warn "未发现 $(basename "$f")"; fi
done

section '审查结束'
printf '结果：%s PASS, %s WARN, %s FAIL\n' "$PASS" "$WARN" "$FAIL"
printf '%s\n' '以上为本机静态审查；Cloudflare/Tunnel 控制台、云防火墙、DNS、证书和实际公网链路未验证。'
[ "$FAIL" -eq 0 ]
