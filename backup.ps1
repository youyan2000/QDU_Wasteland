# backup.ps1 — 触发一次数据备份（H5 自动备份的调度入口）
# 用法：
#   1. 设置环境变量：$env:ADMIN_EMAIL / $env:ADMIN_PASSWORD / $env:QW_BASE_URL(可选，默认 localhost:3000)
#   2. 手动：powershell -ExecutionPolicy Bypass -File backup.ps1
#   3. 每日自动（计划任务）：schtasks /Create /SC DAILY /ST 03:00 /TN "QW-Backup" /TR "powershell -ExecutionPolicy Bypass -File F:\My_Projects\AI_projects\qdu-wasteland\backup.ps1" /F
$ErrorActionPreference = 'Stop'
$base = $env:QW_BASE_URL; if (-not $base) { $base = 'http://localhost:3000' }
$email = $env:ADMIN_EMAIL
$pass  = $env:ADMIN_PASSWORD
if (-not $email -or -not $pass) {
    Write-Error '请先设置 ADMIN_EMAIL / ADMIN_PASSWORD 环境变量（也可用 QW_ADMIN_EMAIL/QW_ADMIN_PASSWORD）'
    exit 1
}
$tmp = Join-Path $env:TEMP ('qw-cookie-' + [guid]::NewGuid() + '.txt')
try {
    $login = @{ email = $email; password = $pass; captcha = ''; captchaId = '' } | ConvertTo-Json
    curl.exe -s -c $tmp -X POST "$base/api/login" -H 'Content-Type: application/json' -H 'User-Agent: qw-backup' --data-binary $login | Out-Null
    $r = curl.exe -s -b $tmp -X POST "$base/api/admin/backup" -H 'User-Agent: qw-backup'
    Write-Output $r
    if ($r -notmatch '"ok":true') { exit 1 }
} finally {
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
}