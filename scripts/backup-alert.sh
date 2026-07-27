#!/bin/sh
set -eu

message="vps-tools backup failed on $(hostname) at $(date -u +%Y-%m-%dT%H:%M:%SZ)"
logger -t vps-tools-backup "$message"

webhook=${VPS_BACKUP_ALERT_WEBHOOK:-}
if [ -n "$webhook" ] && command -v curl >/dev/null 2>&1; then
    curl --fail --silent --show-error --max-time 10 \
        -H 'Content-Type: application/json' \
        --data "{\"text\":\"$message\"}" "$webhook" >/dev/null || \
        logger -t vps-tools-backup "backup alert webhook failed"
fi
echo "$message" >&2
