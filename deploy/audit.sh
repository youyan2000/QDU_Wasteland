#!/usr/bin/env bash
# audit.sh — 服务器状态全面体检（只读，不做任何修改）
# 用法：bash /root/qdu-wasteland/deploy/audit.sh 2>&1 | tee /root/audit-report.txt
# 安全：本脚本只读取信息，不修改、不删除、不重启任何东西

RUN="${QW_RUN_DIR:-/root/qdu-wasteland}"
SRC="${QW_SRC_DIR:-/root/qdu-wasteland-src}"
SERVICE="${QW_SERVICE:-qdu-wasteland}"
SVC_FILE="/etc/systemd/system/${SERVICE}.service"

hr() { echo ""; echo "════════════════════════════════════════════════════════════"; echo "  $1"; echo "════════════════════════════════════════════════════════════"; }

hr "0. 报告时间 / 主机"
date -u '+%Y-%m-%d %H:%M:%S UTC'
echo "主机名: $(hostname)"
echo "内核:   $(uname -r)"
echo "运行时长: $(uptime -p 2>/dev/null || uptime)"
echo "负载: $(cat /proc/loadavg)   CPU 核心: $(nproc)"
echo "公网IP: $(curl -s --max-time 5 ifconfig.me 2>/dev/null || echo '(查询失败)')"

hr "1. 服务状态"
systemctl is-active "$SERVICE" 2>/dev/null | sed 's/^/is-active: /'
systemctl status "$SERVICE" --no-pager 2>/dev/null | head -8

hr "2. 关键：线上运行的程序 vs 磁盘上的程序"
PID="$(systemctl show -p MainPID --value "$SERVICE" 2>/dev/null)"
echo "主进程 PID: $PID"
if [ -n "$PID" ] && [ "$PID" != "0" ]; then
  echo "进程启动时间: $(ps -o lstart= -p "$PID" 2>/dev/null)"
  echo ""
  echo "--- 正在运行的二进制（/proc/$PID/exe）---"
  ls -l "/proc/$PID/exe" 2>/dev/null
  RUNMD5="$(md5sum "/proc/$PID/exe" 2>/dev/null | awk '{print $1}')"
  echo "运行中二进制 MD5: ${RUNMD5:-读取失败}"
  echo "（若上面路径尾部出现 (deleted)，说明磁盘上的程序被替换过但服务没重启）"
fi
echo ""
echo "--- 运行目录的程序文件 ---"
ls -la "$RUN/qdu-wasteland-linux" 2>/dev/null || echo "(不存在)"
DISKMD5="$(md5sum "$RUN/qdu-wasteland-linux" 2>/dev/null | awk '{print $1}')"
echo "磁盘程序 MD5: ${DISKMD5:-读取失败}"
echo ""
echo "--- 源码目录刚编译的程序文件 ---"
ls -la "$SRC/qdu-wasteland-linux" 2>/dev/null || echo "(不存在)"
SRCMD5="$(md5sum "$SRC/qdu-wasteland-linux" 2>/dev/null | awk '{print $1}')"
echo "新编译 MD5: ${SRCMD5:-读取失败}"
echo ""
echo ">>> 结论:"
if [ -n "$RUNMD5" ] && [ -n "$DISKMD5" ] && [ "$RUNMD5" = "$DISKMD5" ]; then
  echo "    ✓ 线上运行的程序 = 磁盘上的程序（已同步）"
else
  echo "    ✗ 线上运行的程序 ≠ 磁盘上的程序 → 需要重启服务才会生效"
fi
if [ -n "$SRCMD5" ] && [ -n "$DISKMD5" ] && [ "$SRCMD5" = "$DISKMD5" ]; then
  echo "    ✓ 已部署的程序 = 最新编译结果"
elif [ -n "$SRCMD5" ]; then
  echo "    ✗ 新编译的程序尚未部署到运行目录"
fi

hr "3. 源码目录 git 状态"
if [ -d "$SRC/.git" ]; then
  cd "$SRC" || exit 1
  echo "远端: $(git config --get remote.origin.url)"
  echo "当前分支: $(git rev-parse --abbrev-ref HEAD)"
  echo "当前提交: $(git log --oneline -1)"
  echo ""
  echo "最近 8 次提交:"
  git log --oneline -8
  echo ""
  echo "未提交的改动:"
  git status --short | head -20
  [ -z "$(git status --short)" ] && echo "(工作区干净)"
else
  echo "✗ $SRC 不是 git 仓库"
fi

hr "4. 前端文件（public/）"
echo "运行目录 public/ 文件数: $(find "$RUN/public" -type f 2>/dev/null | wc -l)"
echo "源码目录 public/ 文件数: $(find "$SRC/public" -type f 2>/dev/null | wc -l)"
echo ""
echo "关键文件对比（时间戳 / 大小）:"
for f in index.html themes.css style.css nav.js; do
  a="$RUN/public/$f"; b="$SRC/public/$f"
  printf "  %-14s 运行: %s\n" "$f" "$(stat -c '%y  %s字节' "$a" 2>/dev/null || echo 缺失)"
  printf "  %-14s 源码: %s\n" "" "$(stat -c '%y  %s字节' "$b" 2>/dev/null || echo 缺失)"
done
echo ""
DIFFCOUNT="$(diff -rq "$SRC/public" "$RUN/public" 2>/dev/null | wc -l)"
echo "运行目录与源码目录 public/ 的差异文件数: $DIFFCOUNT"
[ "$DIFFCOUNT" != "0" ] && diff -rq "$SRC/public" "$RUN/public" 2>/dev/null | head -15

hr "5. 数据库（核心数据）"
ls -la "$RUN"/qdu-auth.db* 2>/dev/null || echo "✗ 未找到数据库文件"
DB="$RUN/qdu-auth.db"
if [ -f "$DB" ]; then
  echo ""
  echo "数据库大小: $(du -h "$DB" | cut -f1)"
  if command -v sqlite3 >/dev/null 2>&1; then
    echo "完整性检查: $(sqlite3 "$DB" 'PRAGMA integrity_check;' 2>&1 | head -3)"
    echo ""
    echo "各表记录数:"
    for t in $(sqlite3 "$DB" "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name;" 2>/dev/null); do
      c="$(sqlite3 "$DB" "SELECT COUNT(*) FROM \"$t\";" 2>/dev/null)"
      printf "  %-28s %s\n" "$t" "$c"
    done
  else
    echo "(未安装 sqlite3，无法检查表结构 —— 建议: apt install -y sqlite3)"
  fi
fi

hr "6. 用户上传文件（uploads/）"
if [ -d "$RUN/uploads" ]; then
  echo "总大小: $(du -sh "$RUN/uploads" 2>/dev/null | cut -f1)"
  echo "文件总数: $(find "$RUN/uploads" -type f 2>/dev/null | wc -l)"
  echo ""
  for d in "$RUN"/uploads/*/; do
    [ -d "$d" ] && printf "  %-30s %s 个文件  %s\n" "$(basename "$d")" "$(find "$d" -type f | wc -l)" "$(du -sh "$d" | cut -f1)"
  done
else
  echo "(uploads/ 目录不存在或为空)"
fi

hr "7. 配置（.env 与环境变量，仅显示变量名，值已脱敏）"
if [ -f "$RUN/.env" ]; then
  echo "环境文件: $RUN/.env （大小 $(stat -c%s "$RUN/.env") 字节，修改时间 $(stat -c%y "$RUN/.env")）"
  echo "包含变量名:"
  grep -oE '^[A-Za-z_][A-Za-z0-9_]*' "$RUN/.env" 2>/dev/null | sed 's/^/  /'
else
  echo "(未找到 $RUN/.env)"
fi
if [ -f "$SVC_FILE" ]; then
  echo ""
  echo "systemd 服务配置 $SVC_FILE:"
  grep -E "WorkingDirectory|ExecStart|EnvironmentFile|Environment=" "$SVC_FILE" 2>/dev/null | sed 's/^/  /'
fi

hr "8. 备份情况"
if [ -d "$RUN/backups" ]; then
  echo "备份目录存在，共 $(ls -1 "$RUN/backups" 2>/dev/null | wc -l) 项，占用 $(du -sh "$RUN/backups" | cut -f1)"
  echo "最近 8 个备份:"
  ls -lt "$RUN/backups" 2>/dev/null | head -9
else
  echo "✗✗✗ 致命：$RUN/backups 目录不存在 —— 服务器上没有任何备份！"
fi
echo ""
echo "定时任务 crontab:"
crontab -l 2>/dev/null | grep -v '^#' | sed 's/^/  /' || echo "  (无定时任务)"
echo ""
echo "系统 cron.d 中相关任务:"
ls -la /etc/cron.d/ 2>/dev/null | tail -5

hr "9. Nginx 与 HTTPS"
systemctl is-active nginx 2>/dev/null | sed 's/^/nginx: /'
echo ""
echo "站点配置文件:"
ls -la /etc/nginx/sites-enabled/ 2>/dev/null
echo ""
echo "反代目标:"
grep -rhE "proxy_pass|server_name|root " /etc/nginx/sites-enabled/ 2>/dev/null | sed 's/^[[:space:]]*/  /' | head -12
echo ""
echo "HTTPS 证书到期时间:"
for c in /etc/letsencrypt/live/*/cert.pem; do
  [ -f "$c" ] && echo "  $c → $(openssl x509 -enddate -noout -in "$c" 2>/dev/null | cut -d= -f2)"
done
echo ""
echo "证书自动续期定时器:"
systemctl list-timers certbot.timer --no-pager 2>/dev/null | head -3

hr "10. 端口监听与防火墙"
ss -tlnp 2>/dev/null | grep -E "LISTEN" | sed 's/^/  /' | head -12
echo ""
if command -v ufw >/dev/null 2>&1; then
  ufw status 2>/dev/null | head -12 | sed 's/^/  /'
else
  echo "  (未安装 ufw)"
fi

hr "11. 磁盘 / 内存"
df -h / | sed 's/^/  /'
echo ""
free -m | sed 's/^/  /'
echo ""
echo "Swap: $(swapon --show 2>/dev/null | tail -n +2 | awk '{print $1, $3}' | tr '\n' ' ')"
[ -z "$(swapon --show 2>/dev/null | tail -n +2)" ] && echo "  ⚠ 无 swap（1核小机器编译时易被 OOM 杀）"

hr "12. ClamAV 病毒扫描（H4 功能依赖）"
if command -v clamscan >/dev/null 2>&1; then
  echo "已安装: $(clamscan --version 2>/dev/null)"
  echo "病毒库更新时间: $(stat -c%y /var/lib/clamav/*.cvd /var/lib/clamav/*.cld 2>/dev/null | head -2)"
  systemctl is-active clamav-freshclam 2>/dev/null | sed 's/^/freshclam: /'
else
  echo "(未安装 ClamAV —— 上传文件的病毒扫描功能未启用)"
fi

hr "13. 最近服务日志（错误排查）"
journalctl -u "$SERVICE" -n 25 --no-pager 2>/dev/null | sed 's/^/  /'

hr "14. 网站可用性自检"
for u in "http://127.0.0.1:3000/" "http://127.0.0.1:3000/themes.css"; do
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 8 "$u" 2>/dev/null)"
  echo "  $u → HTTP $code"
done
echo ""
echo "  外网HTTPS: $(curl -s -o /dev/null -w 'HTTP %{http_code}' --max-time 10 https://qdwasteland.top/ 2>/dev/null || echo '请求失败')"

hr "体检完成"
echo "报告已结束。请把以上完整输出发回分析。"
