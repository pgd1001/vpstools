#!/bin/sh
set -eu

dist_dir=${1:-dist}
[ -d "$dist_dir" ] || { echo "release directory not found: $dist_dir" >&2; exit 1; }
[ -s "$dist_dir/checksums.txt" ] || { echo "checksums.txt is missing or empty" >&2; exit 1; }

if command -v sha256sum >/dev/null 2>&1; then
    (cd "$dist_dir" && sha256sum -c checksums.txt)
elif command -v shasum >/dev/null 2>&1; then
    (cd "$dist_dir" && shasum -a 256 -c checksums.txt)
else
    echo "sha256sum or shasum is required to verify checksums" >&2
    exit 1
fi

sbom_count=$(find "$dist_dir" -maxdepth 1 -type f -name '*.sbom.json' | wc -l | tr -d ' ')
[ "$sbom_count" -gt 0 ] || { echo "no SBOM documents found" >&2; exit 1; }

command -v jq >/dev/null 2>&1 || { echo "jq is required to validate SBOM JSON" >&2; exit 1; }
for sbom in "$dist_dir"/*.sbom.json; do
    jq -e '.bomFormat == "CycloneDX" or .spdxVersion != null' "$sbom" >/dev/null || {
        echo "invalid SBOM document: $sbom" >&2
        exit 1
    }
done

echo "release evidence verified: checksums and $sbom_count SBOM document(s)"
