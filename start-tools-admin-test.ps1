# Start isolated localhost-only admin test instance for tools UI.
# Uses a dedicated SQLite database, never the existing qdu-auth.db.
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$port = 3100
$listen = Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue
if ($listen) { throw "Port $port is already in use; refusing to stop another process." }
$db = Join-Path $PSScriptRoot 'dist\qdu-tools-admin-test.db'
$exe = Join-Path $PSScriptRoot 'dist\wasteland-admin-test.exe'
$user = 'tooladmin'
$email = 'tooladmin@localhost.invalid'
# Do not commit a fixed password. Use TOOL_TEST_PASSWORD when set, otherwise
# generate a throwaway one for this run and print it below.
$password = $env:TOOL_TEST_PASSWORD
if ([string]::IsNullOrWhiteSpace($password)) {
    $alphabet = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789'
    $password = -join (1..20 | ForEach-Object { $alphabet[(Get-Random -Maximum $alphabet.Length)] })
}
$env:PORT = "127.0.0.1:$port"
if (-not $env:CSV_DIR) { $env:CSV_DIR = Join-Path (Split-Path $PSScriptRoot -Parent) '_csv' }
$env:TOOLS_ROOT = Join-Path $PSScriptRoot 'dist\tools'
$env:AUTH_DB = $db
$env:ADMIN_EMAIL = $email
$env:ADMIN_PASSWORD = $password
$env:TOOL_OWNER_USER_ID = '1'
# Test-only CAPTCHA bypass applies only to this isolated localhost process.
$env:DISABLE_CAPTCHA = '1'

go build -o $exe .
if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
Write-Host "Starting isolated local admin test at http://localhost:$port"
Write-Host "Database: $db"
Write-Host "Username: $user"
if (Test-Path $db) {
    Write-Host "Password: (unchanged) this database already exists; the seeded password only applies to a fresh database."
} else {
    Write-Host "Password: $password"
}
Write-Host 'This account exists only in this local test database. CAPTCHA bypass is enabled only for this localhost process.'
Write-Host 'Press Ctrl+C to stop.'
& $exe
if ($LASTEXITCODE -ne 0) { throw "Test server exited with $LASTEXITCODE" }
