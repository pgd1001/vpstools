param(
  [string]$DistDirectory = 'dist'
)

$ErrorActionPreference = 'Stop'
$archives = @(Get-ChildItem -LiteralPath $DistDirectory -Filter '*_windows_*.zip' -File)
if ($archives.Count -eq 0) { throw "No Windows release archives found in $DistDirectory" }

foreach ($archive in $archives) {
  $packageDir = Join-Path ([System.IO.Path]::GetTempPath()) ("vps-tools-release-" + [guid]::NewGuid().ToString('N'))
  try {
    Expand-Archive -LiteralPath $archive.FullName -DestinationPath $packageDir -Force
    foreach ($relativePath in @(
      'api.exe',
      'runner.exe',
      'backup.exe',
      'vps.exe',
      'README.md',
      'deploy/README.md',
      'scripts/install-systemd.sh',
      'scripts/upgrade-systemd.sh',
      'scripts/rollback-systemd.sh',
      'scripts/backup-systemd.sh',
      'scripts/backup-alert.sh',
      'scripts/check-backup-freshness.sh',
      'scripts/healthcheck.sh',
      'scripts/validate-release-layout.ps1',
      'deploy/systemd/api.env.example',
      'deploy/systemd/runner.env.example',
      'deploy/systemd/backup.env.example',
      'deploy/systemd/vps-tools-api.service',
      'deploy/systemd/vps-tools-runner.service',
      'deploy/systemd/vps-tools-backup.service',
      'deploy/systemd/vps-tools-backup.timer',
      'deploy/systemd/vps-tools-backup-alert.service',
      'deploy/systemd/vps-tools-healthcheck.service',
      'deploy/systemd/vps-tools-healthcheck.timer',
      'deploy/systemd/vps-tools-backup-freshness.service',
      'deploy/systemd/vps-tools-backup-freshness.timer'
    )) {
      if (-not (Test-Path -LiteralPath (Join-Path $packageDir $relativePath) -PathType Leaf)) {
        throw "$($archive.Name) is missing $relativePath"
      }
    }
    & (Join-Path $packageDir 'vps.exe') --help | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Packaged vps.exe failed --help in $($archive.Name)" }
    & (Join-Path $packageDir 'backup.exe') -h | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Packaged backup.exe failed -h in $($archive.Name)" }
    Write-Output "release archive layout verified: $($archive.FullName)"
  } finally {
    if (Test-Path -LiteralPath $packageDir) {
      Remove-Item -LiteralPath $packageDir -Recurse -Force -ErrorAction SilentlyContinue
    }
  }
}
