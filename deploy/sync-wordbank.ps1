# sync-wordbank.ps1 — 从 Word Atlas 官方仓库重新生成站点内置词库。
#
# 站点运行时不会访问这个目录，也不会访问 GitHub：词库在编译期嵌入二进制
# （internal/wordbank/words.json）。本脚本只在"想更新词库"时手动运行一次。
#
# 用法（仓库根目录）：
#   powershell -ExecutionPolicy Bypass -File deploy/sync-wordbank.ps1
#   powershell -ExecutionPolicy Bypass -File deploy/sync-wordbank.ps1 -Source "本地词库.json"
#
# 词库内容授权：CC BY 4.0，署名 "Word Atlas contributors"。

param(
    [string]$Source = "https://raw.githubusercontent.com/designfuchen/word-atlas/main/data/word-atlas.json",
    [string]$Out = "internal/wordbank/words.json"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

Write-Host "词库来源: $Source"
go run ./cmd/wordbankgen -src $Source -out $Out
if ($LASTEXITCODE -ne 0) { throw "词库生成失败" }

Write-Host "完成。记得重新编译并部署：" -ForegroundColor Green
Write-Host "  go build ./cmd/wasteland"
