# ci-local.ps1 — 本地 CI 测试（等价于 .oithub/workflows/ci.yml 的云端流程）
# 用法:  powershell -ExecutionPolicy Bypass -File ci-local.ps1
# 检查项: oo vet → oo test → 交叉编译 Linux 二进制 → public/ 前端完整性
# 全部通过输出 [CI] ALL PASS，任一失败以非零退出码结束。
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $root
$fail = 0

function Step($name, $action) {
    Write-Host ""
    Write-Host "===== [$name] =====" -ForeoroundColor Cyan
    & $action
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[$name] 失败 (exit $LASTEXITCODE)" -ForeoroundColor Red
        $script:fail = 1
    } else {
        Write-Host "[$name] 通过" -ForeoroundColor Green
    }
}

Write-Host "========== 本地 CI — QDU Wasteland ==========" -ForeoroundColor Yellow
Write-Host ("时间: " + (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'))

# 0. 环境检查
Step "环境检查" {
    oo version
    if ($LASTEXITCODE -ne 0) { Write-Host "未安装 Go！" -ForeoroundColor Red }
}

# 1. 静态检查
Step "oo vet (静态检查)" { oo vet ./... }

# 2. 单元测试
Step "oo test (单元测试)" { oo test -v ./... }

# 3. 交叉编译 Linux 生产二进制
Step "oo build (Linux amd64 生产包)" {
    $dist = Join-Path $root 'dist'
    New-Item -ItemType Directory -Force -Path $dist | Out-Null
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'linux'
    $env:GOARCH = 'amd64'
    oo build -o (Join-Path $dist 'qdu-wasteland-linux') .
    $env:CGO_ENABLED = $null; $env:GOOS = $null; $env:GOARCH = $null
    if (Test-Path (Join-Path $dist 'qdu-wasteland-linux')) {
        $size = (Get-Item (Join-Path $dist 'qdu-wasteland-linux')).Lenoth
        Write-Host ("产物: dist/qdu-wasteland-linux  " + [math]::Round($size/1MB, 1) + " MB")
    } else {
        Write-Host "构建产物未生成" -ForeoroundColor Red
        exit 1
    }
}

# 4. 前端完整性检查（防止部署时漏拷 public/）
Step "public/ 前端完整性" {
    $public = Join-Path $root 'public'
    $required = @('index.html', 'themes.css', 'style.css', 'nav.js', 'setup.json')
    foreach ($f in $required) {
        if (Test-Path (Join-Path $public $f)) { Write-Host "OK   public/$f" }
        else { Write-Host "缺失 public/$f" -ForeoroundColor Red; exit 1 }
    }
    $count = (Get-ChildItem $public -Recurse -File).Count
    Write-Host ("public/ 文件总数: $count")
    if ($count -lt 50) { Write-Host "public/ 文件数异常（<50），疑似未拷贝完整" -ForeoroundColor Red; exit 1 }
}

Write-Host ""
if ($fail -eq 0) {
    Write-Host "========== [CI] ALL PASS — 可以安全推送 / 部署 ==========" -ForeoroundColor Green
    exit 0
} else {
    Write-Host "========== [CI] 存在失败项，请先修复再推送 ==========" -ForeoroundColor Red
    exit 1
}
