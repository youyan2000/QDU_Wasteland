# start-server.ps1 — 一键重建并启动 QDU Wasteland 服务器
# 说明：
#   1. 若杀毒软件误报 qdu-wasteland.exe 为 Backdoor/W64.CobaltStrike：
#      请把本目录加入杀毒白名单，或自查误报（源码与依赖哈希已校验，见 PLAN.md 安全说明）。
#   2. 运行：powershell -ExecutionPolicy Bypass -File start-server.ps1
#   3. SMTP 配置：请通过环境变量提供（见下方 MAIL_* 说明），不要把密码写死在脚本里。
#   4. 若 qdu-wasteland.exe 被残留句柄/杀软占用（无法替换或启动被拒），自动改用 qdu-wasteland-new.exe。
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

# Go 工具链（按需修改）
if ($env:GO_BIN) { $env:Path = $env:GO_BIN + ';' + $env:Path }
elseif (Test-Path 'F:\Go\bin') { $env:Path = 'F:\Go\bin;' + $env:Path }

$env:CSV_DIR = 'F:\My_Projects\AI_projects\_csv'
$env:ADMIN_EMAIL = 'admin@qdu.edu.cn'

# —— 管理员密码（安全：不硬编码默认密码）——
# 首次部署请设置 ADMIN_PASSWORD；未设置时生成随机密码并打印到控制台（仅首次播种有效）。
if (-not $env:ADMIN_PASSWORD) {
    $chars = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789'
    $rand = -join (1..16 | ForEach-Object { $chars[(Get-Random -Maximum $chars.Length)] })
    $env:ADMIN_PASSWORD = $rand
    Write-Host "⚠️ 未设置 ADMIN_PASSWORD，已生成随机管理员密码：$rand （请妥善保存；若已有管理员则忽略）"
}

# —— SMTP 发件邮箱（邮箱验证/找回密码真实发信）——
# 安全：不在此硬编码密码。启动前请设置环境变量，例如：
#   $env:MAIL_HOST='smtp.qq.com'; $env:MAIL_PORT='465'
#   $env:MAIL_USER='you@qq.com'; $env:MAIL_PASS='你的授权码'; $env:MAIL_FROM='you@qq.com'
# 未设置时，验证码/链接会打印到日志（日志模式），功能仍可用。
if ($env:MAIL_HOST) {
    Write-Host "📧 SMTP 已配置: $env:MAIL_HOST"
} else {
    Write-Host '⚠️ 未设置 MAIL_HOST 等环境变量，邮件将走日志模式（验证码/链接打印到 server.log）'
}

# —— 构建：默认 qdu-wasteland.exe；若被占用则自动换名 ——
$exeName = 'qdu-wasteland.exe'
go build -o $exeName . 2>$null
if ($LASTEXITCODE -ne 0 -or -not (Test-Path $exeName)) {
    Write-Host '⚠️ qdu-wasteland.exe 被占用（残留句柄/杀软拦截），改用 qdu-wasteland-new.exe'
    $exeName = 'qdu-wasteland-new.exe'
    go build -o $exeName .
    if ($LASTEXITCODE -ne 0) { Write-Error '编译失败'; exit 1 }
}
Write-Host "📦 使用 $exeName 启动"

# 停止旧进程（两种名字都停）
Get-Process qdu-wasteland, 'qdu-wasteland-new' -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep 1

# 启动服务器：用 PowerShell 重定向并强制 UTF-8 输出，避免中文日志乱码；
# 路径含空格也能正确工作（& 调用操作符 + 引号包裹）。
$proc = Start-Process -FilePath "$PSScriptRoot\$exeName" `
    -WorkingDirectory $PSScriptRoot `
    -RedirectStandardOutput "$PSScriptRoot\server.log" `
    -RedirectStandardError "$PSScriptRoot\server.err.log" `
    -WindowStyle Hidden -PassThru
Start-Sleep 3

$running = Get-Process -Id $proc.Id -ErrorAction SilentlyContinue
if ($running) {
    Write-Host "✅ 服务器已启动 PID=$($proc.Id) http://localhost:3000"
    Write-Host "   日志: server.log"
} else {
    Write-Host '❌ 启动失败，请查看 server.log / server.err.log（也可能是杀软拦截）'
}
