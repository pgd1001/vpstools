#!/usr/bin/env bash
set -euo pipefail

umask 077
: "${DATABASE_URL:?DATABASE_URL is required}"

backup_dir="${POSTGRES_BACKUP_DIR:-./backups/postgres}"
pg_dump_bin="${PG_DUMP_BIN:-pg_dump}"
pg_restore_bin="${PG_RESTORE_BIN:-pg_restore}"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_file="${backup_dir}/svrtools-${timestamp}.dump"
temporary_file="${backup_file}.tmp.$$"
list_file="${backup_file}.list"
checksum_file="${backup_file}.sha256"

cleanup() {
  rm -f -- "${temporary_file}" "${list_file}"
}
trap cleanup EXIT

mkdir -p -- "${backup_dir}"
"${pg_dump_bin}" --format=custom --no-owner --no-privileges --file "${temporary_file}" "${DATABASE_URL}"
"${pg_restore_bin}" --list "${temporary_file}" > "${list_file}"
test -s "${list_file}"
mv -- "${temporary_file}" "${backup_file}"
(cd "${backup_dir}" && sha256sum -- "$(basename "${backup_file}")" > "$(basename "${checksum_file}")")
chmod 600 -- "${backup_file}" "${checksum_file}"
printf 'PostgreSQL backup created: %s\n' "${backup_file}"
printf 'Checksum: %s\n' "${checksum_file}"
