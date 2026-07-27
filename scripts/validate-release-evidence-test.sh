#!/bin/sh
set -eu

root=$(mktemp -d "${TMPDIR:-/tmp}/vps-tools-release-evidence-test.XXXXXX")
trap 'rm -rf "$root"' EXIT HUP INT TERM
cp docs/release-evidence-template.md "$root/template.md"
sh scripts/validate-release-evidence.sh "$root/template.md" --template >/dev/null

cat > "$root/valid.md" <<'EOF'
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
| Gate | Status | Evidence |
|---|---|---|
| Target-host acceptance output | [x] | evidence/target-host-acceptance.md |
EOF
sh scripts/validate-release-evidence.sh "$root/valid.md" >/dev/null

sed 's/CI run URL: https/CI run URL:/' "$root/valid.md" > "$root/missing.md"
if sh scripts/validate-release-evidence.sh "$root/missing.md" >/dev/null 2>&1; then exit 1; fi
sed 's/| Target-host acceptance output | \[x\] | evidence\/target-host-acceptance.md |/| Target-host acceptance output | [x] | |/' "$root/valid.md" > "$root/unchecked.md"
if sh scripts/validate-release-evidence.sh "$root/unchecked.md" >/dev/null 2>&1; then exit 1; fi

echo "release evidence validator harness passed"
