#!/bin/sh
set -eu

# Exercise the host acceptance gate without requiring systemd, a live API, or
# a real backup. The production-acceptance command remains the target-host
# check. This harness only verifies its pass and fail-closed behaviour.

root=$(mktemp -d "${TMPDIR:-/tmp}/vps-tools-acceptance-test.XXXXXX")
trap 'rm -rf "$root"' EXIT HUP INT TERM
fakebin=$root/bin
mkdir -p "$fakebin"

for command_name in curl systemctl; do
    printf '%s\n' '#!/bin/sh' 'exit 0' > "$fakebin/$command_name"
    chmod 0755 "$fakebin/$command_name"
done

printf '%s\n' '#!/bin/sh' 'exit 0' > "$fakebin/vps"
chmod 0755 "$fakebin/vps"

backup_check=$root/backup-check.sh
printf '%s\n' '#!/bin/sh' 'exit 0' > "$backup_check"
chmod 0755 "$backup_check"

report=$root/acceptance.md
PATH="$fakebin:$PATH" \
VPS_ENV=production \
CLI_BINARY=vps \
BACKUP_CHECK="$backup_check" \
BACKUP_STATUS_FILE="$root/status.json" \
PRODUCTION_EVIDENCE_FILE="$report" \
SKIP_SYSTEMD_CHECK=false \
scripts/production-acceptance.sh >/dev/null

grep -q '^Result, PASS$' "$report"
grep -q 'Token values and authenticated response bodies, omitted' "$report"

if PATH="$fakebin:$PATH" \
    CLI_BINARY=vps \
    BACKUP_CHECK="$backup_check" \
    BACKUP_STATUS_FILE="$root/status.json" \
    scripts/production-acceptance.sh >/dev/null 2>&1; then
    echo "production acceptance should fail without VPS_ENV=production" >&2
    exit 1
fi

echo "production acceptance harness passed"
