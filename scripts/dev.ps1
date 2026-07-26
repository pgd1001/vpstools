# VPS Tools — One-Command Dev Server Launcher
# Usage: .\scripts\dev.ps1
# Requires: Go 1.24+, PowerShell 5.1+

param(
    [switch]$SkipBuild,
    [switch]$WithSSH,
    [string]$APIPort = "8080"
)

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$rootDir = Split-Path -Parent $scriptDir
$binDir = Join-Path $rootDir "bin"

function Write-Color($text, $color = "White") {
    Write-Host $text -ForegroundColor $color
}

function Ensure-Built($name, $path) {
    $exe = Join-Path $binDir "$name.exe"
    $src = Get-Item (Join-Path $rootDir $path) -ErrorAction SilentlyContinue
    $built = Get-Item $exe -ErrorAction SilentlyContinue
    if (-not $SkipBuild -and ((-not $built) -or ($src.LastWriteTime -gt $built.LastWriteTime))) {
        Write-Color "Building $name..." "Cyan"
        go build -o $exe (Join-Path $rootDir $path)
        if ($LASTEXITCODE -ne 0) { throw "Build failed for $name" }
        Write-Color "  $name built." "Green"
    } else {
        Write-Color "  $name is up to date." "DarkGray"
    }
    return $exe
}

Set-Location $rootDir
New-Item -ItemType Directory -Force -Path $binDir | Out-Null

Write-Color "`n=== VPS Tools Dev Server ===`n" "Yellow"

# Build binaries
Ensure-Built "api" "apps/api"
Ensure-Built "vps" "apps/cli"
Ensure-Built "runner" "apps/runner"

# Clean old DB for fresh state (optional: comment out to keep data)
$dbPath = Join-Path $rootDir "svrtools.db"
if (Test-Path $dbPath) {
    Remove-Item $dbPath -Force
    Write-Color "Cleared old database for fresh dev state." "DarkGray"
}

# Start API
Write-Color "`nStarting API server on port $APIPort..." "Cyan"
$apiExe = Join-Path $binDir "api.exe"
$apiEnv = @{
    API_PORT = $APIPort
    VPS_DEV_AUTH = "true"
}
$apiJob = Start-Job -ScriptBlock {
    param($exe, $envVars, $wd)
    Set-Location $wd
    foreach ($k in $envVars.Keys) { [Environment]::SetEnvironmentVariable($k, $envVars[$k], "Process") }
    & $exe
} -ArgumentList $apiExe, $apiEnv, $rootDir

# Wait for API to be ready
$apiReady = $false
for ($i = 0; $i -lt 20; $i++) {
    Start-Sleep -Milliseconds 200
    try {
        $r = Invoke-WebRequest -Uri "http://localhost:$APIPort/api/v1/health" -UseBasicParsing -TimeoutSec 2
        if ($r.StatusCode -eq 200) {
            $apiReady = $true
            break
        }
    } catch { }
}
if (-not $apiReady) {
    Write-Color "API failed to start. Logs:" "Red"
    Receive-Job $apiJob
    Remove-Job $apiJob -Force
    exit 1
}
Write-Color "API is ready." "Green"

# Start runner
Write-Color "Starting runner in simulate mode..." "Cyan"
$runnerExe = Join-Path $binDir "runner.exe"
$runnerEnv = @{
    API_URL = "http://localhost:$APIPort"
    SIMULATE = "true"
    VPS_DEV_AUTH = "true"
}
$runnerJob = Start-Job -ScriptBlock {
    param($exe, $envVars, $wd)
    Set-Location $wd
    foreach ($k in $envVars.Keys) { [Environment]::SetEnvironmentVariable($k, $envVars[$k], "Process") }
    & $exe
} -ArgumentList $runnerExe, $runnerEnv, $rootDir

Start-Sleep -Seconds 1
Write-Color "Runner started." "Green"

Write-Color "`n=== Dev server is running ===`n" "Yellow"
Write-Color "API:      http://localhost:$APIPort" "White"
Write-Color "Health:   http://localhost:$APIPort/api/v1/health" "White"
Write-Color "Database: $dbPath" "White"
Write-Color ""
Write-Color "Quick test commands:" "White"
Write-Color "  .\bin\vps.exe whoami" "DarkGray"
Write-Color "  .\bin\vps.exe server list" "DarkGray"
Write-Color "  .\bin\vps.exe runbook list" "DarkGray"
Write-Color "  .\bin\vps.exe exec server:demo -- uptime" "DarkGray"
Write-Color "  .\bin\vps.exe exec list --mine" "DarkGray"
Write-Color "  .\bin\vps.exe audit search --limit 5" "DarkGray"
Write-Color "  .\bin\vps.exe tui" "DarkGray"
Write-Color ""
Write-Color "Users (set VPS_USER env):" "White"
Write-Color "  user_senior  (Senior Engineer)" "DarkGray"
Write-Color "  user_junior  (Junior Engineer — can only run runbooks)" "DarkGray"
Write-Color "  user_auditor (Auditor — read-only)" "DarkGray"
Write-Color ""
Write-Color "Press Ctrl+C to stop all services." "Yellow"

# Trap Ctrl+C
$running = $true
[Console]::TreatControlCAsInput = $true

while ($running) {
    if ([Console]::KeyAvailable) {
        $key = [Console]::ReadKey($true)
        if ($key.Key -eq "C" -and $key.Modifiers -band [System.ConsoleModifiers]::Control) {
            $running = $false
        }
    }
    # Check if jobs crashed
    if ($apiJob.State -eq "Failed") {
        Write-Color "`nAPI crashed:" "Red"
        Receive-Job $apiJob | Write-Host
    }
    if ($runnerJob.State -eq "Failed") {
        Write-Color "`nRunner crashed:" "Red"
        Receive-Job $runnerJob | Write-Host
    }
    Start-Sleep -Milliseconds 200
}

Write-Color "`nShutting down..." "Yellow"
Stop-Job $apiJob -ErrorAction SilentlyContinue
Stop-Job $runnerJob -ErrorAction SilentlyContinue
Remove-Job $apiJob -Force -ErrorAction SilentlyContinue
Remove-Job $runnerJob -Force -ErrorAction SilentlyContinue
Write-Color "Done." "Green"
