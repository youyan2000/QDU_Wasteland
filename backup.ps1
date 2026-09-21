# backup.ps1 — 从电脑触发【服务器端本地备份】（通过 SSH，不需要管理员密码）
#
# 背景：安全修复后，登录接口不再对任何邮箱豁免验证码/限流，
#       因此旧版"用管理员账号登录再调 /api/admin/backup"的方式已废弃。
#       现在改为直接通过 SSH 执行服务器上的备份脚本（deploy/backup.sh）。
#
# 用法：
#   1. 设置服务器地址：$env:QW_SSH_HOST = 'root@你的服务器IP'
#   2. 手动触发一次：powershell -ExecutionPolicy Bypass -File backup.ps1
#   3. 可选每日计划任务（服务器端 cron 已在每日 3:00 备份，此处一般不需要）：
#      schtasks /Create /SC DAILY /ST 03:00 /TN "QW-Backup" /TR "powershell -ExecutionPolicy Bypass -File F:\My_Projects\Wasteland\qdu-wasteland\backup.ps1" /F
#
# 备份产物在服务器 /root/qdu-wasteland/backups/（qdu-时间戳.db + uploads-时间戳.zip，保留最近14份）
$ErrorActionPreference = 'Stop'

$sshHost = $env:QW_SSH_HOST
if (-not $sshHost) {
    Write-Error '请设置 QW_SSH_HOST（如 root@你的服务器IP）'
    exit 1
}

Write-Output "正在通过 SSH 触发服务器端备份: $sshHost"

# 直接执行服务器上的本地备份脚本（sqlite3 VACUUM INTO 快照 + uploads 打包 + 自动验证 + 轮转）
ssh $sshHost "bash /root/qdu-wasteland/deploy/backup.sh"
if ($LASTEXITCODE -ne 0) {
    Write-Error "服务器端备份失败（ssh 退出码 $LASTEXITCODE）"
    exit 1
}

Write-Output '备份完成。服务器上的产物位于 /root/qdu-wasteland/backups/'
Write-Output '如需把备份拉回本机，请运行 pull-backup.ps1'
