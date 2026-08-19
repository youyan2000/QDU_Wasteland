#!/usr/bin/env bash
# enable-clamav.sh — 正式启用 ClamAV 病毒扫描（H4 部署项）
# 在部署机上执行一次（需要 root）：
#   sudo bash enable-clamav.sh
# 完成后上传的文件都会经过 ClamAV 扫描；日志里出现 [ClamAV] 即生效。
# 说明：应用代码已内置 ClamAV 集成（virus.go，环境变量 CLAMAV_CMD 启用），
#       本脚本负责：安装 ClamAV → 更新病毒库 → 把 CLAMAV_CMD 注入服务环境 → EICAR 自检。
set -euo pipefail

SERVICE_NAME="${QW_SERVICE:-qdu-wasteland}"
ENV_FILE="${QW_ENV_FILE:-/etc/qdu-wasteland.env}"

echo "==> [1/5] 安装 ClamAV"
apt-get update -y
DEBIAN_FRONTEND=noninteractive apt-get install -y clamav clamav-daemon

echo "==> [2/5] 更新病毒库（首次可能较慢，约几分钟）"
systemctl stop clamav-freshclam 2>/dev/null || true
freshclam || echo "警告: freshclam 更新失败，可稍后手动运行 freshclam"
systemctl start clamav-freshclam 2>/dev/null || true

echo "==> [3/5] 注入 CLAMAV_CMD 到服务环境文件 ${ENV_FILE}"
mkdir -p "$(dirname "$ENV_FILE")"
if ! grep -q '^CLAMAV_CMD=' "$ENV_FILE" 2>/dev/null; then
    echo 'CLAMAV_CMD=/usr/bin/clamscan' >> "$ENV_FILE"
fi
cat "$ENV_FILE"

echo "==> [4/5] 确认 systemd 单元读取环境文件（若已配置请忽略）"
if [ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]; then
    if ! grep -q 'EnvironmentFile' "/etc/systemd/system/${SERVICE_NAME}.service"; then
        echo "警告: ${SERVICE_NAME}.service 未含 EnvironmentFile=${ENV_FILE}，请手动添加："
        echo "  [Service]"
        echo "  EnvironmentFile=${ENV_FILE}"
    fi
    systemctl daemon-reload
fi

echo "==> [5/5] EICAR 自检（应输出: EICAR-Test-File ... FOUND）"
EICAR='X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*'
printf '%s' "$EICAR" > /tmp/qdu-eicar.txt
clamscan --no-summary /tmp/qdu-eicar.txt || true
rm -f /tmp/qdu-eicar.txt

echo "==> 完成。重启服务使环境变量生效："
echo "    systemctl restart ${SERVICE_NAME}"
echo "    验证：上传一个 EICAR 测试文件应被拒绝；server.err.log 会记录 [ClamAV] 扫描日志"
