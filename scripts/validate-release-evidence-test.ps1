$ErrorActionPreference = 'Stop'
$root = Join-Path ([System.IO.Path]::GetTempPath()) ('vps-tools-release-evidence-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root | Out-Null
try {
  $valid = Join-Path $root 'valid.md'
  @'
- Version, exact release tag: v1.2.3
- Git commit, full SHA: 0123456789012345678901234567890123456789
- CI run URL: https://github.com/example/repo/actions/runs/123
- Checksums file, path or URL: PASS, dist/checksums.txt, SHA-256 verified
- SBOM document(s), path or URL: PASS, dist/app.sbom.json, CycloneDX validated
- Target-host acceptance output: PASS, evidence/target-host-acceptance.md
- Backup and restore result: PASS, evidence/backup-restore.md
- Measured RPO: 15 minutes, evidence/backup-restore.md
- Measured RTO: 22 minutes, evidence/backup-restore.md
- Rollback result: PASS, v1.2.2 restored and doctor passed, evidence/rollback.md
- Identity-provider verification result: PASS, OIDC login passed for test subject, evidence/oidc.md
- Accepted residual risks, or `None`: None
'@ | Set-Content -LiteralPath $valid

  & (Join-Path $PSScriptRoot 'validate-release-evidence.ps1') -EvidenceFile $valid | Out-Null

  $invalid = Join-Path $root 'invalid.md'
  (Get-Content -Raw -LiteralPath $valid).Replace('Target-host acceptance output: PASS', 'Target-host acceptance output: [ ] not verified') | Set-Content -LiteralPath $invalid
  try {
    & (Join-Path $PSScriptRoot 'validate-release-evidence.ps1') -EvidenceFile $invalid | Out-Null
    throw 'incomplete target-host evidence was accepted'
  } catch {
    if ($_.Exception.Message -eq 'incomplete target-host evidence was accepted') { throw }
  }
  Write-Output 'release evidence PowerShell validator tests passed'
} finally {
  Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
}
