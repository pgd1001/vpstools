#!/bin/sh
set -eu

backup_root=${BACKUP_ROOT:-/var/lib/vps-tools/backups}
retention_days=${BACKUP_RETENTION_DAYS:-14}
replication_dir=${BACKUP_REPLICATION_DIR:-}
status_file=${BACKUP_STATUS_FILE:-$backup_root/backup-status.json}
backup_binary=${BACKUP_BINARY:-/opt/vps-tools/current/backup}
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
output="$backup_root/$timestamp"

case "$backup_root" in
    /*) ;;
    *) echo "BACKUP_ROOT must be an absolute path" >&2; exit 2;;
esac
[ "$backup_root" != "/" ] || { echo "BACKUP_ROOT cannot be the filesystem root" >&2; exit 2; }

case "$retention_days" in
    ''|*[!0-9]*) echo "BACKUP_RETENTION_DAYS must be a non-negative integer" >&2; exit 2 ;;
esac

install -d -m 0700 "$backup_root"
"$backup_binary" -db "${DATABASE_URL:-/var/lib/vps-tools/svrtools.db}" \
    -artifacts "${ARTIFACTS_DIR:-/var/lib/vps-tools/data/artifacts}" -output "$output"
"$backup_binary" -mode verify -input "$output"
ln -sfn "$output" "$backup_root/latest"

if [ -n "$replication_dir" ]; then
    case "$replication_dir" in
        /*) ;;
        *) echo "BACKUP_REPLICATION_DIR must be an absolute path" >&2; exit 2;;
    esac
    [ "$replication_dir" != "/" ] || { echo "BACKUP_REPLICATION_DIR cannot be the filesystem root" >&2; exit 2; }
    case "$replication_dir" in
        /var/lib/vps-tools/*) ;;
        *) echo "BACKUP_REPLICATION_DIR must be under /var/lib/vps-tools for the systemd sandbox; mount the separate destination there" >&2; exit 2;;
    esac
    install -d -m 0700 "$replication_dir"
    replication_tmp="$replication_dir/.replication-$timestamp"
    rm -rf -- "$replication_tmp"
    cp -a -- "$output" "$replication_tmp"
    "$backup_binary" -mode verify -input "$replication_tmp"
    mv -T -- "$replication_tmp" "$replication_dir/$timestamp"
    ln -sfn "$replication_dir/$timestamp" "$replication_dir/latest"
fi

manifest_checksum=""
if command -v sha256sum >/dev/null 2>&1; then
    manifest_checksum=$(sha256sum "$output/manifest.json" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    manifest_checksum=$(shasum -a 256 "$output/manifest.json" | awk '{print $1}')
fi
artifact_files=0
if [ -d "$output/artifacts" ]; then
    artifact_files=$(find "$output/artifacts" -type f | wc -l | tr -d ' ')
fi
replication_status=false
[ -n "$replication_dir" ] && replication_status=true
status_dir=$(dirname "$status_file")
install -d -m 0700 "$status_dir"
status_tmp="$status_dir/.backup-status-$timestamp"
cat > "$status_tmp" <<EOF
{"backup_created_at":"$timestamp","verified":true,"replicated":$replication_status,"artifact_files":$artifact_files,"manifest_sha256":"$manifest_checksum","retention_days":$retention_days}
EOF
chmod 0600 "$status_tmp"
mv -f -- "$status_tmp" "$status_file"

if [ "$retention_days" -gt 0 ]; then
    find "$backup_root" -mindepth 1 -maxdepth 1 -type d -name '20????????T??????Z' \
        -mtime "+$retention_days" -exec rm -rf -- {} +
fi
echo "backup completed: $output"
