[CmdletBinding()]
param([string]$DistDir = "dist")

$ErrorActionPreference = "Stop"
$checksumFile = Join-Path $DistDir "checksums.txt"
if (-not (Test-Path -LiteralPath $checksumFile)) { throw "checksums.txt is missing" }

$distRoot = (Resolve-Path -LiteralPath $DistDir).Path
$entries = @(Get-Content -LiteralPath $checksumFile | Where-Object { $_.Trim().Length -gt 0 })
if ($entries.Count -eq 0) { throw "checksums.txt contains no SHA-256 entries" }
$seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
foreach ($entry in $entries) {
    if ($entry -notmatch '^([0-9a-fA-F]{64})\s+\*?(.+)$') { throw "malformed checksum entry: $entry" }
    $expected = $Matches[1].ToLowerInvariant()
    $relativeName = $Matches[2].Trim()
    if ([System.IO.Path]::IsPathRooted($relativeName)) { throw "absolute checksum target is not allowed: $relativeName" }
    if (-not $seen.Add($relativeName)) { throw "duplicate checksum target: $relativeName" }
    $file = Join-Path $distRoot $relativeName
    $resolvedFile = [System.IO.Path]::GetFullPath($file)
    if (-not $resolvedFile.StartsWith($distRoot.TrimEnd('\') + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) { throw "checksum target escapes distribution directory: $relativeName" }
    if (-not (Test-Path -LiteralPath $resolvedFile -PathType Leaf)) { throw "checksum target is missing: $relativeName" }
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $resolvedFile).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "checksum mismatch: $relativeName" }
}

$sboms = @(Get-ChildItem -LiteralPath $DistDir -Filter "*.sbom.json" -File)
if ($sboms.Count -eq 0) { throw "no SBOM documents found" }
foreach ($sbom in $sboms) {
    $document = Get-Content -Raw -LiteralPath $sbom.FullName | ConvertFrom-Json
    if (-not ($document.bomFormat -eq "CycloneDX" -or $null -ne $document.spdxVersion)) {
        throw "invalid SBOM document: $($sbom.Name)"
    }
}
Write-Output "release evidence verified: checksums and $($sboms.Count) SBOM document(s)"
