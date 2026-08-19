# pull-backup.ps1 — 服务器完整备份拉取到电脑（方向③）
# 功能：
#   1. 服务器端已由 backup.go + deploy/backup.sh 生成 backups/（数据库快照 + uploads 打包，轮转14份）
#   2. 本脚本用 scp 把服务器 backups/ 拉到电脑本地目录
#   3. 可选：推送到你的 GitHub 私有仓库（qdu-server-backup）
# 定时：Windows 计划任务（见文末示例命令）
#
# 用法（先设置环境变量或改下方配置）：
#   powershell -ExecutionPolicy Bypass -File pull-backup.ps1

$ErrorActionPreference = 'Stop'

# ---------- 配置（改这里）----------
$SSH_HOST  = $env:QW_SSH_HOST       # 服务器地址，如 'root@207.148.106.155' 或 'root@你的IP'
$SSH_PORT  = if ($env:QW_SSH_PORT) { $env:QW_SSH_PORT } else { '22' }
$REMOTE_DIR = if ($env:QW_REMOTE_BACKUP_DIR) { $env:QW_REMOTE_BACKUP_DIR } else { '/root/qdu-wasteland/backups/' }  # 服务器备份目录
$LOCAL_DIR  = if ($env:QW_LOCAL_BACKUP_DIR) { $env:QW_LOCAL_BACKUP_DIR } else { 'D:\QDU-backups' }                 # 电脑备份目录

# 可选：推送到 GitHub 私有仓库（留空则跳过）
$GIT_REPO   = $env:QW_BACKUP_GIT_REPO    # 如 'https://github.com/youyan2000/qdu-server-backup.git'
$GIT_TOKEN  = $env:QW_BACKUP_GIT_TOKEN   # GitHub token（repo 权限）

if (-not $SSH_HOST) {
    Write-Error '请设置 QW_SSH_HOST（如 root@服务器IP）'; exit 1
}

# ---------- 1. 建本地目录 ----------
New-Item -ItemType Directory -Force -Path $LOCAL_DIR | Out-Null
Write-Host "备份到: $LOCAL_DIR"

# ---------- 2. 用 scp 拉取服务器备份 ----------
Write-Host "==> scp 拉取 $REMOTE_DIR → $LOCAL_DIR"
# 检查 scp 是否可用（OpenSSH）
if (-not (Get-Command scp -ErrorAction SilentlyContinue)) {
    Write-Error '未找到 scp（Windows 需安装 OpenSSH 客户端，见 https://learn.microsoft.com/windows-server/administration/openssh/openssh_install_firstuse）'
    exit 1
}
# scp -P 端口 -r 递归。首次会提示输入服务器密码/密钥。
& scp -P $SSH_PORT -r "$SSH_HOST`:$REMOTE_DIR*" $LOCAL_DIR 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Error "scp 失败（exit $LASTEXITCODE）。检查 SSH_HOST/端口/密码，或改用密钥认证。"
    exit 1
}
Write-Host "✅ 拉取完成"

# ---------- 3. 清理本地超过保留份数的旧备份（可选，保留最近 N 份）----------
$KEEP = 30
$all = Get-ChildItem $LOCAL_DIR -Recurse -File | Sort-Object LastWriteTime -Descending
if ($all.Count -gt $KEEP) {
    $all | Select-Object -Skip $KEEP | Remove-Item -Force -ErrorAction SilentlyContinue
    Write-Host "清理了 $($all.Count - $KEEP) 个旧文件（保留最近 $KEEP 份）"
}

# ---------- 4. 可选：推送到 GitHub 私有仓库 ----------
if ($GIT_REPO -and $GIT_TOKEN) {
    Write-Host "==> 推送到私有仓库 $GIT_REPO"
    $gitDir = Join-Path $env:TEMP 'qdu-backup-git'
    # 把 https://github.com/xxx 转成带 token 的克隆地址
    $cloneUrl = $GIT_REPO -replace '^https://', "https://x-access-token:$GIT_TOKEN@"
    if (-not (Test-Path (Join-Path $gitDir '.git'))) {
        git clone $cloneUrl $gitDir 2>&1 | Out-Null
    }
    Set-Location $gitDir
    git config user.name 'qdu-backup'
    git config user.email 'backup@qdu.local'
    # 同步备份文件进仓库
    Copy-Item (Join-Path $LOCAL_DIR '*') $gitDir -Recurse -Force
    git add -A 2>&1 | Out-Null
    if (-not (git diff --cached --quiet)) {
        git commit -m "服务器备份 $(Get-Date -Format 'yyyy-MM-dd HHmm')" 2>&1 | Out-Null
        git push origin main 2>&1 | Out-Null
        Write-Host "✅ 已推送私有仓库"
    } else {
        Write-Host "  无新备份，跳过"
    }
} else {
    Write-Host "（未配置 QW_BACKUP_GIT_REPO，跳过私有仓库推送）"
}

Write-Host "`n✅ 全部完成: $(Get-Date)"
Write-Host '提示：可配置 Windows 计划任务每日自动执行：'
Write-Host '  schtasks /Create /SC DAILY /ST 05:00 /TN QW-PullBackup /TR "powershell -ExecutionPolicy Bypass -File F:\My_Projects\AI_projects\qdu-wasteland\pull-backup.ps1" /F'
