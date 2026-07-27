#!/usr/bin/env bash
set -euo pipefail

# Restore and verify an extended deployment recovery record. This restores
# PostgreSQL and verifies the S3 objects named by the retained manifest. Set
# RESTORE_ARTIFACTS_DIR to also materialise a verified local fallback copy.
umask 077
: "${DATABASE_URL:?DATABASE_URL is required}"
: "${EXTENDED_BACKUP:?EXTENDED_BACKUP is required}"
: "${CONFIRM_RESTORE:?Set CONFIRM_RESTORE=YES to restore an extended backup}"
[[ "${CONFIRM_RESTORE}" == YES ]] || {
  printf 'Refusing restore. Set CONFIRM_RESTORE=YES after validating the target database.\n' >&2
  exit 1
}

postgres_restore_script="${POSTGRES_RESTORE_SCRIPT:-./scripts/postgres-restore.sh}"
artifact_migrate_bin="${ARTIFACT_MIGRATE_BINARY:-artifact-migrate}"
record_manifest="${EXTENDED_BACKUP}/manifest.json"
[[ -f "${postgres_restore_script}" ]] || { printf 'PostgreSQL restore helper is missing: %s\n' "${postgres_restore_script}" >&2; exit 1; }
command -v "${artifact_migrate_bin}" >/dev/null 2>&1 || { printf 'artifact-migrate binary is required to verify S3 artefacts: %s\n' "${artifact_migrate_bin}" >&2; exit 1; }
[[ -f "${record_manifest}" ]] || { printf 'Extended backup manifest does not exist: %s\n' "${record_manifest}" >&2; exit 1; }

postgres_dump="$(sed -n 's/^[[:space:]]*"postgres_dump":[[:space:]]*"\([^"]*\)".*/\1/p' "${record_manifest}")"
artifact_manifest="$(sed -n 's/^[[:space:]]*"artifact_manifest":[[:space:]]*"\([^"]*\)".*/\1/p' "${record_manifest}")"
[[ -n "${postgres_dump}" && -n "${artifact_manifest}" ]] || { printf 'Extended backup manifest is incomplete: %s\n' "${record_manifest}" >&2; exit 1; }
postgres_dump="${EXTENDED_BACKUP}/${postgres_dump}"
artifact_manifest="${EXTENDED_BACKUP}/${artifact_manifest}"
[[ -f "${postgres_dump}" ]] || { printf 'PostgreSQL dump does not exist: %s\n' "${postgres_dump}" >&2; exit 1; }
[[ -f "${artifact_manifest}" ]] || { printf 'S3 artifact manifest does not exist: %s\n' "${artifact_manifest}" >&2; exit 1; }

checksum_file="${postgres_dump}.sha256"
[[ -f "${checksum_file}" ]] || { printf 'PostgreSQL dump checksum does not exist: %s\n' "${checksum_file}" >&2; exit 1; }
(cd "$(dirname "${checksum_file}")" && sha256sum --check "$(basename "${checksum_file}")")
(cd "$(dirname "${artifact_manifest}")" && sha256sum --check manifest.json.sha256)
CONFIRM_RESTORE=YES DATABASE_URL="${DATABASE_URL}" BACKUP_FILE="${postgres_dump}" bash "${postgres_restore_script}"
"${artifact_migrate_bin}" -verify-manifest "${artifact_manifest}"

if [[ -n "${RESTORE_ARTIFACTS_DIR:-}" ]]; then
  "${artifact_migrate_bin}" -restore-manifest "${artifact_manifest}" -restore-artifacts "${RESTORE_ARTIFACTS_DIR}"
fi
printf 'Extended backup restored and S3 artefacts verified: %s\n' "${EXTENDED_BACKUP}"
