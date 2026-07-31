#!/bin/sh
set -eu

[ "$(id -u)" -eq 0 ] || { echo "run as root" >&2; exit 1; }
[ $# -ge 1 ] && [ $# -le 2 ] || { echo "Usage: $0 RELEASE_DIR [VERSION]" >&2; exit 2; }
release_dir=$1
version=${2:-$(basename "$(cd "$release_dir" && pwd)")}
case "$version" in *[!A-Za-z0-9._-]*) echo "invalid version: $version" >&2; exit 2;; esac
[ -x "$release_dir/api" ] && [ -x "$release_dir/runner" ] && [ -x "$release_dir/backup" ] && [ -x "$release_dir/vps" ] || { echo "release must contain executable api, runner, backup, and vps" >&2; exit 1; }

target=/opt/vps-tools/releases/$version
[ ! -e "$target" ] || { echo "release already exists: $target" >&2; exit 1; }
install -d -m 0755 /opt/vps-tools/releases
tmp="$target.install.$$"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
install -d -m 0755 "$tmp"
install -m 0755 "$release_dir/api" "$tmp/api"
install -m 0755 "$release_dir/runner" "$tmp/runner"
install -m 0755 "$release_dir/backup" "$tmp/backup"
install -m 0755 "$release_dir/vps" "$tmp/vps"
mv "$tmp" "$target"
trap - EXIT HUP INT TERM

# A host upgraded from a release that predates per-server SSH identity has no
# credential directory, because only the installer created it. Create it here
# too so the runner's configured SSH_CREDENTIALS_DIR exists and an operator has
# somewhere to place keys. It is left empty: the upgrade cannot know which key
# belongs to which host, and the runner refuses a server whose credential does
# not resolve rather than connecting unverified.
if [ ! -d /etc/vps-tools/ssh-credentials ]; then
    install -d -m 0700 /etc/vps-tools/ssh-credentials
    chown vps-tools:vps-tools /etc/vps-tools/ssh-credentials 2>/dev/null || true
fi

old_target=$(readlink -f /opt/vps-tools/current 2>/dev/null || true)
case "$old_target" in
    "") ;;
    /opt/vps-tools/releases/*) [ -x "$old_target/api" ] && [ -x "$old_target/runner" ] && [ -x "$old_target/backup" ] && [ -x "$old_target/vps" ] || { echo "current release is incomplete: $old_target" >&2; exit 1; } ;;
    *) echo "current release resolves outside /opt/vps-tools/releases: $old_target" >&2; exit 1;;
esac
if [ -n "$old_target" ] && [ -x /usr/local/libexec/vps-tools/backup-systemd.sh ]; then
    echo "Creating and verifying a pre-upgrade backup"
    /usr/local/libexec/vps-tools/backup-systemd.sh
fi
systemctl stop vps-tools-runner.service vps-tools-api.service
ln -sfn "$target" /opt/vps-tools/current
if systemctl start vps-tools-api.service && /usr/local/libexec/vps-tools/healthcheck.sh --api-only && systemctl start vps-tools-runner.service; then
    echo "Upgraded to $version"
    [ -z "$old_target" ] || echo "Rollback target remains $old_target"
else
    echo "Upgrade failed, restoring previous release" >&2
    [ -n "$old_target" ] || { echo "no previous release available" >&2; exit 1; }
    systemctl stop vps-tools-runner.service vps-tools-api.service || true
    ln -sfn "$old_target" /opt/vps-tools/current
    systemctl start vps-tools-api.service
    systemctl start vps-tools-runner.service
    exit 1
fi
