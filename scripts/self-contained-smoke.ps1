param(
  [string]$ApiBinary = (Join-Path $PSScriptRoot '..\bin\api.exe'),
  [string]$RunnerBinary = (Join-Path $PSScriptRoot '..\bin\runner.exe'),
  [string]$BackupBinary = (Join-Path $PSScriptRoot '..\bin\backup.exe'),
  [string]$CliBinary = (Join-Path $PSScriptRoot '..\bin\vps.exe'),
  [int]$Port = 18080
)

$ErrorActionPreference = 'Stop'

foreach ($binary in @($ApiBinary, $RunnerBinary, $BackupBinary, $CliBinary)) {
  if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) { throw "Required binary not found: $binary" }
}

$root = Join-Path $env:TEMP ("vps-tools-smoke-" + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $root -Force
$api = $null
$runner = $null
$restoredApi = $null
$old = @{}
foreach ($name in @('DATABASE_URL','VPS_ARTIFACTS_DIR','ARTIFACTS_DIR','BACKUP_ENCRYPTION_KEY','API_PORT','VPS_ENV','VPS_DEV_AUTH','VPS_API_URL','VPS_USER','VPS_API_TOKEN','API_URL','SIMULATE','VPS_RUNNER_TOKEN','JOB_SIGNING_KEY','RUNNER_NAME','RUNNER_HEALTH_ADDR','VPS_MCP_SMOKE_LIVE')) {
  $old[$name] = [Environment]::GetEnvironmentVariable($name)
}

try {
  $env:DATABASE_URL = Join-Path $root 'svrtools.db'
  $env:VPS_ARTIFACTS_DIR = Join-Path $root 'artifacts'
  $env:ARTIFACTS_DIR = $env:VPS_ARTIFACTS_DIR
  $env:BACKUP_ENCRYPTION_KEY = 'YmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmI='
  $env:API_PORT = [string]$Port
  $env:VPS_ENV = 'development'
  $env:VPS_DEV_AUTH = 'true'
  # The API signs every dispatched job and the runner refuses any job it cannot
  # verify, so both processes need the same key.
  $env:JOB_SIGNING_KEY = 'self-contained-smoke-signing-key-32ch'
  $env:VPS_API_URL = "http://127.0.0.1:$Port"
  $env:VPS_USER = 'user_senior'
  $env:VPS_API_TOKEN = ''

  $api = Start-Process -FilePath (Resolve-Path $ApiBinary) -WorkingDirectory $root -RedirectStandardOutput (Join-Path $root 'api.out') -RedirectStandardError (Join-Path $root 'api.err') -PassThru
  $ready = $false
  $readyResponse = $null
  for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Milliseconds 250
    try {
      $health = Invoke-RestMethod -Uri "$env:VPS_API_URL/api/v1/ready" -TimeoutSec 2
      if ($health.status -eq 'ready' -and $health.artifacts -eq 'ok') { $readyResponse = $health; $ready = $true; break }
    } catch { }
  }
  if (-not $ready) { throw "API did not become ready. See $root\api.err" }

  $apiTokenResponse = Invoke-RestMethod -Method Post -Uri "$env:VPS_API_URL/api/v1/auth/tokens" -Headers @{ 'X-VPS-User' = 'user_senior' } -ContentType 'application/json' -Body '{"name":"self-contained-smoke","user_id":"user_senior","expires_in":3600}'
  if (-not $apiTokenResponse.token) { throw 'API token issuance validation failed' }
  $env:VPS_API_TOKEN = $apiTokenResponse.token
  $tokenIdentity = Invoke-RestMethod -Uri "$env:VPS_API_URL/api/v1/whoami" -Headers @{ Authorization = "Bearer $env:VPS_API_TOKEN" }
  if ($tokenIdentity.user_id -ne 'user_senior') { throw 'Bearer token identity validation failed' }
  & $CliBinary doctor --api-url $env:VPS_API_URL | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'CLI doctor validation failed' }

  $metrics = Invoke-WebRequest -Uri "$env:VPS_API_URL/metrics" -Headers @{ 'X-VPS-User' = 'user_senior' } -UseBasicParsing
  if ($metrics.StatusCode -ne 200 -or $metrics.Content -notmatch 'svrtools_api_requests_total') { throw 'Metrics endpoint validation failed' }
  $pause = Invoke-RestMethod -Method Post -Uri "$env:VPS_API_URL/api/v1/automation/pause" -Headers @{ 'X-VPS-User' = 'user_senior' } -ContentType 'application/json' -Body '{"reason":"self-contained smoke test"}'
  if (-not $pause.paused) { throw 'Automation pause validation failed' }
  $resume = Invoke-RestMethod -Method Post -Uri "$env:VPS_API_URL/api/v1/automation/resume" -Headers @{ 'X-VPS-User' = 'user_senior' } -ContentType 'application/json' -Body '{}'
  if ($resume.paused) { throw 'Automation resume validation failed' }

  $scheduleBody = '{"name":"self-contained-smoke-schedule","runbook_name":"check-uptime","target":"server:srv_demo","reason":"self-contained smoke test","params":{},"interval_seconds":3600}'
  $scheduleCreated = Invoke-RestMethod -Method Post -Uri "$env:VPS_API_URL/api/v1/schedules" -Headers @{ 'X-VPS-User' = 'user_senior' } -ContentType 'application/json' -Body $scheduleBody
  if ($scheduleCreated.status -ne 'created') { throw 'Schedule creation validation failed' }
  $schedules = Invoke-RestMethod -Uri "$env:VPS_API_URL/api/v1/schedules" -Headers @{ 'X-VPS-User' = 'user_senior' }
  $schedule = $schedules.schedules | Where-Object { $_.name -eq 'self-contained-smoke-schedule' } | Select-Object -First 1
  if (-not $schedule) { throw 'Schedule listing validation failed' }
  $scheduleDisabled = Invoke-RestMethod -Method Delete -Uri "$env:VPS_API_URL/api/v1/schedules/$($schedule.id)" -Headers @{ 'X-VPS-User' = 'user_senior' }
  if ($scheduleDisabled.status -ne 'disabled') { throw 'Schedule disable validation failed' }

  $mcpDir = Join-Path $PSScriptRoot '..\mcp'
  if (Test-Path -LiteralPath (Join-Path $mcpDir 'node_modules') -PathType Container) {
    $env:VPS_MCP_SMOKE_LIVE = 'true'
    $env:VPS_API_URL = "http://127.0.0.1:$Port"
    $env:VPS_USER = ''
    Push-Location $mcpDir
    try {
      npm run smoke
      if ($LASTEXITCODE -ne 0) { throw 'Live MCP smoke failed' }
    } finally {
      Pop-Location
    }
  }

  $tokenResponse = Invoke-RestMethod -Method Post -Uri "$env:VPS_API_URL/api/v1/runners/registration-token" -Headers @{ 'X-VPS-User' = 'user_senior' }
  $env:API_URL = $env:VPS_API_URL
  $env:SIMULATE = 'true'
  $env:RUNNER_HEALTH_ADDR = "127.0.0.1:$($Port + 2)"
  $env:VPS_RUNNER_TOKEN = $tokenResponse.registration_token
  $env:RUNNER_NAME = 'self-contained-smoke-runner'
  $runner = Start-Process -FilePath (Resolve-Path $RunnerBinary) -WorkingDirectory $root -RedirectStandardOutput (Join-Path $root 'runner.out') -RedirectStandardError (Join-Path $root 'runner.err') -PassThru
  $runnerReady = $false
  for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Milliseconds 250
    try {
      $runnerHealth = Invoke-RestMethod -Uri "http://127.0.0.1:$($Port + 2)/health" -TimeoutSec 2
      if ($runnerHealth.status -eq 'healthy') { $runnerReady = $true; break }
    } catch { }
  }
  if (-not $runnerReady) { throw "Runner health endpoint did not become ready. See $root\runner.err" }

  $env:VPS_API_URL = $env:VPS_API_URL
  $execution = & (Resolve-Path $CliBinary) exec server:srv_demo --reason 'self-contained smoke test' --wait --timeout 30 -- uptime 2>&1
  if ($LASTEXITCODE -ne 0) { throw "CLI execution failed: $($execution -join ' ')" }
  if (-not (($execution -join "`n") -match 'succeeded')) { throw "CLI execution did not report success: $($execution -join ' ')" }

  $backupDir = Join-Path $root 'backups\latest'
  $replicaDir = Join-Path $root 'replica\latest'
  $restoreDb = Join-Path $root 'restore\svrtools.db'
  $restoreArtifacts = Join-Path $root 'restore\artifacts'
  $null = New-Item -ItemType Directory -Path (Split-Path $backupDir), (Split-Path $replicaDir) -Force
  & (Resolve-Path $BackupBinary) -db $env:DATABASE_URL -artifacts $env:VPS_ARTIFACTS_DIR -output $backupDir
  if ($LASTEXITCODE -ne 0) { throw 'Backup creation failed' }
  & (Resolve-Path $BackupBinary) -mode verify -input $backupDir
  if ($LASTEXITCODE -ne 0) { throw 'Backup verification failed' }
  Copy-Item -LiteralPath $backupDir -Destination $replicaDir -Recurse
  & (Resolve-Path $BackupBinary) -mode verify -input $replicaDir
  if ($LASTEXITCODE -ne 0) { throw 'Replicated backup verification failed' }
  & (Resolve-Path $BackupBinary) -mode restore -input $replicaDir -db $restoreDb -artifacts $restoreArtifacts
  if ($LASTEXITCODE -ne 0) { throw 'Backup restore failed' }
  if (-not (Test-Path -LiteralPath $restoreDb -PathType Leaf)) { throw 'Restored database is missing' }
  if (-not (Test-Path -LiteralPath $restoreArtifacts -PathType Container)) { throw 'Restored artefact directory is missing' }
  $env:DATABASE_URL = $restoreDb
  $env:VPS_ARTIFACTS_DIR = $restoreArtifacts
  $env:ARTIFACTS_DIR = $restoreArtifacts
  $env:API_PORT = [string]($Port + 1)
  $env:VPS_API_URL = "http://127.0.0.1:$($Port + 1)"
  $env:VPS_ENV = 'development'
  $env:VPS_DEV_AUTH = 'true'
  $restoredApi = Start-Process -FilePath (Resolve-Path $ApiBinary) -WorkingDirectory (Split-Path $restoreDb) -RedirectStandardOutput (Join-Path $root 'restored-api.out') -RedirectStandardError (Join-Path $root 'restored-api.err') -PassThru
  $restoredReady = $false
  for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Milliseconds 250
    try {
      $restoredHealth = Invoke-RestMethod -Uri "$env:VPS_API_URL/api/v1/ready" -TimeoutSec 2
      if ($restoredHealth.status -eq 'ready' -and $restoredHealth.artifacts -eq 'ok') { $restoredReady = $true; break }
    } catch { }
  }
  if (-not $restoredReady) { throw "Restored API did not become ready. See $root\restored-api.err" }
  $restoredIdentity = Invoke-RestMethod -Uri "$env:VPS_API_URL/api/v1/whoami" -Headers @{ 'X-VPS-User' = 'user_senior' }
  if ($restoredIdentity.user_id -ne 'user_senior') { throw 'Restored identity validation failed' }
  $restoredExecutions = Invoke-RestMethod -Uri "$env:VPS_API_URL/api/v1/executions" -Headers @{ 'X-VPS-User' = 'user_senior' }
  if (-not $restoredExecutions.executions) { throw 'Restored execution history is missing' }
  $restoredAudit = Invoke-RestMethod -Uri "$env:VPS_API_URL/api/v1/audit" -Headers @{ 'X-VPS-User' = 'user_senior' }
  if (-not $restoredAudit.events) { throw 'Restored audit history is missing' }
  $restoredAuditVerification = Invoke-RestMethod -Uri "$env:VPS_API_URL/api/v1/audit/verify" -Headers @{ 'X-VPS-User' = 'user_senior' }
  if (-not $restoredAuditVerification.valid -or $restoredAuditVerification.checked_events -lt 1) { throw 'Restored audit hash-chain verification failed' }
  Write-Output "self-contained smoke and backup restore passed. Temporary state was removed."
} finally {
  if ($runner -and -not $runner.HasExited) { Stop-Process -Id $runner.Id -Force -ErrorAction SilentlyContinue }
  if ($api -and -not $api.HasExited) { Stop-Process -Id $api.Id -Force -ErrorAction SilentlyContinue }
  if ($restoredApi -and -not $restoredApi.HasExited) { Stop-Process -Id $restoredApi.Id -Force -ErrorAction SilentlyContinue }
  foreach ($name in $old.Keys) { [Environment]::SetEnvironmentVariable($name, $old[$name]) }
  if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue }
}
