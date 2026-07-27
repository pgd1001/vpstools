#!/bin/sh
set -eu

status_file=${BACKUP_STATUS_FILE:-/var/lib/vps-tools/backups/backup-status.json}
backup_root=${BACKUP_ROOT:-/var/lib/vps-tools/backups}
replication_dir=${BACKUP_REPLICATION_DIR:-}
backup_binary=${BACKUP_BINARY:-/opt/vps-tools/current/backup}
max_age=${BACKUP_MAX_AGE_SECONDS:-129600}
[ -f "$status_file" ] || { echo "backup status file is missing: $status_file" >&2; exit 1; }
case "$max_age" in ''|*[!0-9]*) echo "BACKUP_MAX_AGE_SECONDS must be an integer" >&2; exit 2;; esac

mtime=$(stat -c %Y "$status_file" 2>/dev/null || stat -f %m "$status_file")
now=$(date +%s)
age=$((now - mtime))
[ "$age" -le "$max_age" ] || { echo "backup status is stale: age=${age}s max=${max_age}s" >&2; exit 1; }

grep -Eq '^\{"backup_created_at":"[0-9]{8}T[0-9]{6}Z","verified":true,"replicated":(true|false),"artifact_files":[0-9]+,"manifest_sha256":"[0-9a-f]{64}","retention_days":[0-9]+\}$' "$status_file" || {
    echo "backup status file is malformed or unverified: $status_file" >&2
    exit 1
}

manifest_hash=$(sed -n 's/.*"manifest_sha256":"\([0-9a-f]\{64\}\)".*/\1/p' "$status_file")
latest_dir=$(readlink -f "$backup_root/latest" 2>/dev/null || true)
[ -n "$latest_dir" ] && [ -d "$latest_dir" ] || { echo "latest backup is missing: $backup_root/latest" >&2; exit 1; }
[ -f "$latest_dir/manifest.json" ] || { echo "latest backup manifest is missing: $latest_dir/manifest.json" >&2; exit 1; }

if command -v sha256sum >/dev/null 2>&1; then
    actual_hash=$(sha256sum "$latest_dir/manifest.json" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    actual_hash=$(shasum -a 256 "$latest_dir/manifest.json" | awk '{print $1}')
else
    echo "sha256sum or shasum is required to verify the latest backup" >&2
    exit 1
fi
[ "$actual_hash" = "$manifest_hash" ] || { echo "latest backup manifest does not match backup status" >&2; exit 1; }
[ -x "$backup_binary" ] || { echo "backup verifier is missing or not executable: $backup_binary" >&2; exit 1; }
"$backup_binary" -mode verify -input "$latest_dir" >/dev/null

if [ -n "$replication_dir" ]; then
    replicated_dir=$(readlink -f "$replication_dir/latest" 2>/dev/null || true)
    [ -n "$replicated_dir" ] && [ -d "$replicated_dir" ] || { echo "replicated latest backup is missing: $replication_dir/latest" >&2; exit 1; }
    [ -f "$replicated_dir/manifest.json" ] || { echo "replicated backup manifest is missing: $replicated_dir/manifest.json" >&2; exit 1; }
    replicated_hash=$(sha256sum "$replicated_dir/manifest.json" 2>/dev/null | awk '{print $1}' || shasum -a 256 "$replicated_dir/manifest.json" | awk '{print $1}')
    [ "$replicated_hash" = "$manifest_hash" ] || { echo "replicated backup manifest does not match backup status" >&2; exit 1; }
    "$backup_binary" -mode verify -input "$replicated_dir" >/dev/null
fi
echo "backup freshness check passed: age=${age}s"
