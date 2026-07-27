#!/bin/sh
set -eu

[ "$(id -u)" -eq 0 ] || { echo "run as root" >&2; exit 1; }
[ $# -eq 1 ] || { echo "Usage: $0 VERSION_OR_RELEASE_DIR" >&2; exit 2; }
target=$1
case "$target" in
    /*) case "$target" in /opt/vps-tools/releases/*) ;; *) echo "rollback target must be under /opt/vps-tools/releases" >&2; exit 2;; esac;;
    *) target=/opt/vps-tools/releases/$target;;
esac
[ -e "$target" ] || { echo "release does not exist: $target" >&2; exit 1; }
target=$(readlink -f "$target")
case "$target" in
    /opt/vps-tools/releases/*) ;;
    *) echo "rollback target resolved outside /opt/vps-tools/releases: $target" >&2; exit 2;;
esac
[ -x "$target/api" ] && [ -x "$target/runner" ] && [ -x "$target/backup" ] && [ -x "$target/vps" ] || { echo "release is missing executable api, runner, backup, or vps: $target" >&2; exit 1; }
if [ ! -x "$target/backup" ]; then
    echo "warning: release has no backup binary; stop or disable vps-tools-backup.timer after rollback" >&2
fi

systemctl stop vps-tools-runner.service vps-tools-api.service
ln -sfn "$target" /opt/vps-tools/current
systemctl start vps-tools-api.service
/usr/local/libexec/vps-tools/healthcheck.sh --api-only
systemctl start vps-tools-runner.service
echo "Rolled back to $(basename "$target")"
