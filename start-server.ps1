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

$env:CSV_DIR = 'F:\My_Projects\Wasteland\_csv'
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

# —— 构建：入口在 cmd/wasteland/（2026-10 重构后仓库根目录已没有 main 包）——
$repoDir = $PSScriptRoot
$exeName = Join-Path $repoDir 'qdu-wasteland.exe'
go build -o $exeName ./cmd/wasteland
if ($LASTEXITCODE -ne 0 -or -not (Test-Path $exeName)) {
    # 旧进程占着 exe 时换名重试：这是"文件占用"，不是路径问题
    Write-Host '⚠️ 未产出 qdu-wasteland.exe，改用 qdu-wasteland-new.exe 重试'
    $exeName = Join-Path $repoDir 'qdu-wasteland-new.exe'
    go build -o $exeName ./cmd/wasteland
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path $exeName)) {
        Write-Host ''
        Write-Host '❌ 构建失败。先看上面的 go 输出，常见原因：'
        Write-Host '   · no Go files / package ... is not in std → 入口必须是 ./cmd/wasteland（本脚本已用）'
        Write-Host '   · Access is denied，或 exe 刚生成就被删掉 → 杀毒软件（火绒/Defender）拦截，'
        Write-Host "     请把 $repoDir 加入信任区后重试"
        exit 1
    }
}
Write-Host "📦 使用 $exeName 启动"

# 停止旧进程（两种名字都停）
Get-Process qdu-wasteland, 'qdu-wasteland-new' -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep 1

# 启动服务器。APP_DIR 显式指向仓库根：即使工作目录不对，public/ 也能找到
# （缺它时站点会只剩工具页能开、其余页面全 404，很难看出是路径问题）。
$env:APP_DIR = $repoDir
$logOut = Join-Path $repoDir 'server.log'
$logErr = Join-Path $repoDir 'server.err.log'
$proc = Start-Process -FilePath $exeName `
    -WorkingDirectory $repoDir `
    -RedirectStandardOutput $logOut `
    -RedirectStandardError $logErr `
    -WindowStyle Hidden -PassThru
Start-Sleep 3

$running = Get-Process -Id $proc.Id -ErrorAction SilentlyContinue
if ($running) {
    Write-Host "✅ 服务器已启动 PID=$($proc.Id) http://localhost:3000"
    Write-Host "   日志: $logOut"
} else {
    Write-Host '❌ 进程启动后立刻退出。请依次检查：'
    Write-Host "   1) 错误日志 $logErr（端口被占、CSV_DIR 不存在等）"
    if (Test-Path $logErr) { Get-Content $logErr -Tail 12 -Encoding UTF8 | ForEach-Object { Write-Host "      $_" } }
    Write-Host '   2) 若日志为空且 exe 已消失 → 杀毒软件拦截（火绒/Defender），把仓库目录加入信任区'
}
