# Start the complete LOCAL website, including /tool/ and NeXTJLC.
# Does not deploy, publish, replace online files, or reset accounts.
param(
    [string]$NextJlcPath = 'F:\My_Projects\Open_Source\nextjlc',
    [int]$Port = 3000
)
$ErrorActionPreference = 'Stop'
$projectDir = $PSScriptRoot
if (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue) {
    throw "Port $Port is already in use. Open http://localhost:$Port/tool/ if the local website is running, or stop that server first."
}
$bunExe = Join-Path $env:USERPROFILE '.bun\bin\bun.exe'
if (-not (Test-Path $bunExe)) { $bunExe = (Get-Command bun -ErrorAction Stop).Source }
if (-not (Test-Path (Join-Path $NextJlcPath 'pkg\nextjlc_bg.wasm'))) {
    throw 'NeXTJLC pkg is missing. Build WASM in the NeXTJLC project first.'
}
Push-Location $NextJlcPath
try {
    & $bunExe run typecheck
    if ($LASTEXITCODE -ne 0) { throw 'NeXTJLC typecheck failed.' }
    & $bunExe run build:web
    if ($LASTEXITCODE -ne 0) { throw 'NeXTJLC build failed.' }
} finally { Pop-Location }
$bundlePath = Join-Path $projectDir 'dist\tools\nextjlc'
New-Item -ItemType Directory -Force $bundlePath | Out-Null
Copy-Item (Join-Path $NextJlcPath 'dist\*') $bundlePath -Recurse -Force
$exePath = Join-Path $projectDir 'dist\wasteland-integrated.exe'
Push-Location $projectDir
$savedEnv = @{}
foreach ($name in 'PORT','CSV_DIR','TOOLS_ROOT','ADMIN_EMAIL','ADMIN_PASSWORD') {
    $savedEnv[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
try {
    & go build -o $exePath .
    if ($LASTEXITCODE -ne 0) { throw 'Local website build failed.' }
    $env:PORT = "127.0.0.1:$Port"
    $env:CSV_DIR = Join-Path (Split-Path $projectDir -Parent) '_csv'
    $env:TOOLS_ROOT = Join-Path $projectDir 'dist\tools'
    # Existing local accounts are used; do not seed or alter administrator credentials.
    $env:ADMIN_EMAIL = ''
    $env:ADMIN_PASSWORD = ''
    Write-Host "Starting the complete local website. After course loading, open http://localhost:$Port/tool/"
    Write-Host 'Press Ctrl+C to stop. No online deployment is performed.'
    & $exePath
    if ($LASTEXITCODE -ne 0) { throw "Local website exited with code $LASTEXITCODE." }
} finally {
    foreach ($name in $savedEnv.Keys) {
        [Environment]::SetEnvironmentVariable($name, $savedEnv[$name], 'Process')
    }
    Pop-Location
}
