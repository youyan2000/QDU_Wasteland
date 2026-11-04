# install-git-hooks.ps1 — 启用仓库自带的 git 钩子（提交前卫生检查）
# 用法（仓库根目录）：powershell -ExecutionPolicy Bypass -File deploy/install-git-hooks.ps1

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if (-not (Test-Path '.githooks/pre-commit')) {
    Write-Host '找不到 .githooks/pre-commit' -ForegroundColor Red
    exit 1
}

git config core.hooksPath .githooks
if ($LASTEXITCODE -ne 0) { Write-Host '设置 core.hooksPath 失败' -ForegroundColor Red; exit 1 }

Write-Host '已启用 git 钩子：core.hooksPath = .githooks' -ForegroundColor Green
Write-Host '验证：随便 stage 一张图片到仓库里，git commit 应被拒绝。'
Write-Host '绕过（不推荐）：git commit --no-verify'
