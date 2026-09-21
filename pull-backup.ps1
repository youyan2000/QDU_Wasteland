# pull-backup.ps1 — 服务器完整备份拉取到电脑（方向③：异地备份）
#
# 作用：把服务器 /root/qdu-wasteland/backups/ 里的备份（数据库快照 + uploads 打包）
#       拉到电脑本地目录，并可选推送到 GitHub 私有仓库。
#       意义：服务器硬盘损坏时，数据与服务器备份会一起丢失 —— 本地副本才是真正的保险。
#
# 用法（在电脑上执行）：
#   $env:QW_SSH_HOST = 'root@你的服务器IP'
#   $env:QW_LOCAL_BACKUP_DIR = 'F:\My_Projects\Wasteland\backups'      # 可省略，默认 F:\My_Projects\Wasteland\backups
#   powershell -ExecutionPolicy Bypass -File pull-backup.ps1
#   （首次会提示输入服务器密码）
#
# 定时（可选，见文末；无人值守需先配置 SSH 密钥免密登录）
$ErrorActionPreference = 'Stop'

# ---------- 配置 ----------
$SSH_HOST   = $env:QW_SSH_HOST
$SSH_PORT   = if ($env:QW_SSH_PORT) { $env:QW_SSH_PORT } else { '22' }
$REMOTE_DIR = if ($env:QW_REMOTE_BACKUP_DIR) { $env:QW_REMOTE_BACKUP_DIR } else { '/root/qdu-wasteland/backups/' }
$LOCAL_DIR  = if ($env:QW_LOCAL_BACKUP_DIR) { $env:QW_LOCAL_BACKUP_DIR } else { 'F:\My_Projects\Wasteland\backups' }
$KEEP       = 15   # 本地保留最近 N 批（每批 = 1 个数据库快照 + 1 个 uploads 包）

# 可选：推送到 GitHub 私有仓库（留空则跳过）
$GIT_REPO   = $env:QW_BACKUP_GIT_REPO
$GIT_TOKEN  = $env:QW_BACKUP_GIT_TOKEN

if (-not $SSH_HOST) {
    Write-Error '请设置 QW_SSH_HOST（如 root@你的服务器IP）'; exit 1
}
if (-not (Get-Command scp -ErrorAction SilentlyContinue)) {
    Write-Error '未找到 scp（Windows 需安装 OpenSSH 客户端）'
    exit 1
}

# ---------- 1. 本地目录 ----------
New-Item -ItemType Directory -Force -Path $LOCAL_DIR | Out-Null
Write-Host "备份到: $LOCAL_DIR"

# ---------- 2. scp 拉取 ----------
Write-Host "==> scp 拉取 $REMOTE_DIR → $LOCAL_DIR"
& scp -P $SSH_PORT -r "$SSH_HOST`:$REMOTE_DIR*" $LOCAL_DIR 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Error "scp 失败（exit $LASTEXITCODE）。检查 QW_SSH_HOST / 端口 / 密码，或改用 SSH 密钥。"
    exit 1
}
Write-Host "✅ 拉取完成"

# ---------- 3. 清理本地旧备份：按「批次」保留最近 KEEP 批 ----------
# 按时间戳成对清理，避免出现"有库无图 / 有图无库"的残缺批次；
# 且只识别时间戳命名的文件，绝不触碰手工备份（如 qdu-auth.db / -wal / -shm）。
$stamps = @(
    Get-ChildItem $LOCAL_DIR -File -ErrorAction SilentlyContinue | ForEach-Object {
        if ($_.Name -match '^qdu-(\d{4}-\d{2}-\d{2}-\d{6})\.db$') { $Matches[1] }
        elseif ($_.Name -match '^uploads-(\d{4}-\d{2}-\d{2}-\d{6})\.(zip|tar\.gz)$') { $Matches[1] }
    } | Sort-Object -Unique
)
if ($stamps.Count -gt $KEEP) {
    $del = $stamps | Select-Object -First ($stamps.Count - $KEEP)
    foreach ($s in $del) {
        foreach ($name in @("qdu-$s.db", "uploads-$s.zip", "uploads-$s.tar.gz")) {
            $p = Join-Path $LOCAL_DIR $name
            if (Test-Path $p) {
                Remove-Item $p -Force
                Write-Host "  清理旧备份: $name"
            }
        }
    }
}
$left = @(Get-ChildItem $LOCAL_DIR -File -ErrorAction SilentlyContinue).Count
Write-Host "  本地现有 $left 个备份文件，共 $($stamps.Count) 批（保留上限 $KEEP 批）"

# ---------- 4. 可选：推送到 GitHub 私有仓库 ----------
if ($GIT_REPO -and $GIT_TOKEN) {
    Write-Host "==> 推送到私有仓库 $GIT_REPO"
    $gitDir = Join-Path $env:TEMP 'qdu-backup-git'
    $cloneUrl = $GIT_REPO -replace '^https://', "https://x-access-token:$GIT_TOKEN@"
    if (-not (Test-Path (Join-Path $gitDir '.git'))) {
        git clone $cloneUrl $gitDir 2>&1 | Out-Null
    }
    Set-Location $gitDir
    git config user.name 'qdu-backup'
    git config user.email 'backup@qdu.local'
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
Write-Host '提示：可配置 Windows 计划任务每日自动执行（无人值守需先配置 SSH 密钥免密登录）：'
Write-Host '  schtasks /Create /SC DAILY /ST 05:00 /TN QW-PullBackup /TR "powershell -ExecutionPolicy Bypass -File F:\My_Projects\Wasteland\qdu-wasteland\pull-backup.ps1" /F'
