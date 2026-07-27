#!/usr/bin/env bash
set -euo pipefail

# Create one recovery record for an extended deployment. PostgreSQL is copied
# into the record. S3 artefacts remain in S3, so the retained artefact
# manifest is copied and verified as part of the same record.
umask 077
: "${DATABASE_URL:?DATABASE_URL is required}"
: "${ARTIFACT_MANIFEST:?ARTIFACT_MANIFEST is required}"

backup_dir="${EXTENDED_BACKUP_DIR:-./backups/extended}"
postgres_backup_script="${POSTGRES_BACKUP_SCRIPT:-./scripts/postgres-backup.sh}"
artifact_migrate_bin="${ARTIFACT_MIGRATE_BINARY:-artifact-migrate}"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
destination="${backup_dir}/svrtools-${timestamp}"
temporary="${destination}.tmp.$$"

cleanup() {
  rm -rf -- "${temporary}"
}
trap cleanup EXIT

[[ -f "${postgres_backup_script}" ]] || {
  printf 'PostgreSQL backup helper is missing: %s\n' "${postgres_backup_script}" >&2
  exit 1
}
command -v "${artifact_migrate_bin}" >/dev/null 2>&1 || {
  printf 'artifact-migrate binary is required to verify S3 artefacts: %s\n' "${artifact_migrate_bin}" >&2
  exit 1
}
[[ -f "${ARTIFACT_MANIFEST}" ]] || {
  printf 'S3 artifact manifest does not exist: %s\n' "${ARTIFACT_MANIFEST}" >&2
  exit 1
}

mkdir -p -- "${backup_dir}" "${temporary}/postgres" "${temporary}/artifacts"
POSTGRES_BACKUP_DIR="${temporary}/postgres" bash "${postgres_backup_script}"
artifact_migrate_output="${temporary}/artifacts/artifact-migrate-verify.txt"
"${artifact_migrate_bin}" -verify-manifest "${ARTIFACT_MANIFEST}" >"${artifact_migrate_output}"
cp -- "${ARTIFACT_MANIFEST}" "${temporary}/artifacts/manifest.json"
(cd "${temporary}/artifacts" && sha256sum -- manifest.json > manifest.json.sha256)

postgres_dump="$(find "${temporary}/postgres" -maxdepth 1 -type f -name '*.dump' -print -quit)"
[[ -n "${postgres_dump}" ]] || {
  printf 'PostgreSQL backup helper did not produce a dump\n' >&2
  exit 1
}
postgres_dump_name="$(basename "${postgres_dump}")"
postgres_checksum="${postgres_dump}.sha256"
[[ -f "${postgres_checksum}" ]] || {
  printf 'PostgreSQL backup helper did not produce a checksum\n' >&2
  exit 1
}

cat > "${temporary}/manifest.json" <<EOF
{
  "version": 1,
  "created_at": "$(date -u '+%Y-%m-%dT%H:%M:%SZ')",
  "postgres_dump": "postgres/${postgres_dump_name}",
  "postgres_checksum": "postgres/${postgres_dump_name}.sha256",
  "artifact_manifest": "artifacts/manifest.json",
  "artifact_manifest_checksum": "artifacts/manifest.json.sha256",
  "artifact_verification_log": "artifacts/artifact-migrate-verify.txt",
  "recovery_notes": "PostgreSQL is restored from the dump. S3 objects are verified from the retained manifest. Keep the S3 encryption key and bucket retention policy with this record."
}
EOF
chmod 600 -- "${temporary}/manifest.json"
[[ ! -e "${destination}" ]] || {
  printf 'Backup destination already exists: %s\n' "${destination}" >&2
  exit 1
}
mv -- "${temporary}" "${destination}"
printf 'Extended backup created: %s\n' "${destination}"
