#!/usr/bin/env bash
set -euo pipefail

umask 077
: "${DATABASE_URL:?DATABASE_URL is required}"
: "${BACKUP_FILE:?BACKUP_FILE is required}"
: "${CONFIRM_RESTORE:?Set CONFIRM_RESTORE=YES to restore a PostgreSQL backup}"
if [[ "${CONFIRM_RESTORE}" != "YES" ]]; then
  printf 'Refusing restore. Set CONFIRM_RESTORE=YES after validating the target database.\n' >&2
  exit 1
fi

pg_restore_bin="${PG_RESTORE_BIN:-pg_restore}"
psql_bin="${PSQL_BIN:-psql}"
if [[ ! -f "${BACKUP_FILE}" ]]; then
  printf 'Backup file does not exist: %s\n' "${BACKUP_FILE}" >&2
  exit 1
fi

checksum_file="${BACKUP_FILE}.sha256"
if [[ -f "${checksum_file}" ]]; then
  (cd "$(dirname "${checksum_file}")" && sha256sum --check "$(basename "${checksum_file}")")
fi
"${pg_restore_bin}" --list "${BACKUP_FILE}" > /dev/null
"${pg_restore_bin}" --clean --if-exists --exit-on-error --no-owner --no-privileges --dbname "${DATABASE_URL}" "${BACKUP_FILE}"
"${psql_bin}" "${DATABASE_URL}" --no-psqlrc --set ON_ERROR_STOP=1 --command 'SELECT 1;' > /dev/null
printf 'PostgreSQL backup restored and connection verified: %s\n' "${BACKUP_FILE}"
