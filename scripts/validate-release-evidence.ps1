[CmdletBinding()]
param(
  [string]$EvidenceFile = 'docs/release-evidence-template.md',
  [switch]$Template
)

$ErrorActionPreference = 'Stop'
if (-not (Test-Path -LiteralPath $EvidenceFile -PathType Leaf)) { throw "release evidence file not found: $EvidenceFile" }
$content = Get-Content -LiteralPath $EvidenceFile
$requiredFields = @(
  'Version, exact release tag:', 'Git commit, full SHA:', 'CI run URL:',
  'Checksums file, path or URL:', 'SBOM document(s), path or URL:',
  'Target-host acceptance output:', 'Backup and restore result:',
  'Measured RPO:', 'Measured RTO:', 'Rollback result:',
  'Identity-provider verification result:', 'Accepted residual risks, or'
)

foreach ($field in $requiredFields) {
  $line = $content | Where-Object { $_ -match ('^' + [regex]::Escape("- $field")) } | Select-Object -First 1
  if ($null -eq $line) { throw "missing release evidence field label: $field" }
  $value = if ($null -eq $line) { '' } else { ($line -replace "^- [^:]+:\s*", '').Trim() }
  if (-not $Template -and [string]::IsNullOrWhiteSpace($value)) { throw "missing release evidence field: $field" }
  if (-not $Template -and $value -match '^(TBD|N/A|not run|todo|<.*>)$') { throw "placeholder release evidence field: $field" }
}

if (-not $Template) {
  $commit = ($content | Where-Object { $_ -like '- Git commit, full SHA:*' } | Select-Object -First 1) -replace '^- [^:]+:\s*', ''
  if ($commit -notmatch '(?i)[0-9a-f]{40}') { throw 'Git commit must include a 40-character commit hash' }
  $ciUrl = ($content | Where-Object { $_ -like '- CI run URL:*' } | Select-Object -First 1) -replace '^- [^:]+:\s*', ''
  if ($ciUrl -notmatch '(?i)https://[^\s]+/actions/runs/[0-9]+') { throw 'CI run URL must point to a GitHub Actions run' }

  foreach ($field in @('Checksums file, path or URL:', 'SBOM document(s), path or URL:', 'Target-host acceptance output:', 'Backup and restore result:', 'Rollback result:', 'Identity-provider verification result:')) {
    $line = $content | Where-Object { $_ -like "- $field*" } | Select-Object -First 1
    $value = ($line -replace '^- [^:]+:\s*', '').Trim()
    if ($value -notmatch '(?i)(^|[^\w])PASS([^\w]|$)') { throw "gate is not recorded as PASS: $field" }
    if ($value -notmatch '(?i)(^|[\s,])(https?://|[\w.-]+/|[\w.-]+\.(txt|json|md|log))') { throw "gate has no retained evidence path or URL: $field" }
  }
  foreach ($field in @('Measured RPO:', 'Measured RTO:')) {
    $line = $content | Where-Object { $_ -like "- $field*" } | Select-Object -First 1
    $value = ($line -replace '^- [^:]+:\s*', '').Trim()
    if ($value -notmatch '(?i)[0-9]+\s*(second|minute|hour|day|week)s?') { throw "measured duration is missing or invalid: $field" }
  }
}

if (-not $Template) {
  $commit = (($content | Where-Object { $_ -like '- Git commit, full SHA:*' } | Select-Object -First 1) -replace "^- [^:]+:\s*", '').Trim()
  if ($commit -notmatch '^[0-9a-fA-F]{40}$') { throw 'Git commit must be a full 40-character SHA' }
  $ciUrl = (($content | Where-Object { $_ -like '- CI run URL:*' } | Select-Object -First 1) -replace "^- [^:]+:\s*", '').Trim()
  if ($ciUrl -notmatch '^https?://') { throw 'CI run URL must be an HTTP(S) URL' }
}

$inTable = $false
foreach ($line in $content) {
  if ($line -match '^\|\s*Gate\s*\|') { $inTable = $true; continue }
  if ($inTable -and $line -match '^\|') {
    $cells = $line.Trim('|').Split('|')
    if ($cells.Count -ge 3 -and $cells[1] -match '\[[xX]\]' -and $cells[2].Trim() -notmatch '[^\s-]') {
      throw "checked environment-only gate has no evidence: $($cells[0].Trim())"
    }
  }
}
Write-Output "release evidence validated: $EvidenceFile"
