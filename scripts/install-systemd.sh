#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)

usage() {
    echo "Usage: $0 RELEASE_DIR [VERSION]" >&2
    echo "Installs the api, runner, backup, and vps binaries from RELEASE_DIR into /opt/vps-tools." >&2
    exit 2
}

[ "$(id -u)" -eq 0 ] || { echo "run as root" >&2; exit 1; }
[ $# -ge 1 ] && [ $# -le 2 ] || usage
release_dir=$1
version=${2:-$(basename "$(cd "$release_dir" && pwd)")}
case "$version" in *[!A-Za-z0-9._-]*) echo "invalid version: $version" >&2; exit 2;; esac
[ -x "$release_dir/api" ] && [ -x "$release_dir/runner" ] && [ -x "$release_dir/backup" ] && [ -x "$release_dir/vps" ] || { echo "release must contain executable api, runner, backup, and vps" >&2; exit 1; }

install -d -m 0755 /opt/vps-tools/releases /etc/vps-tools /var/lib/vps-tools/data/artifacts /usr/local/libexec/vps-tools
if ! getent group vps-tools >/dev/null 2>&1; then groupadd --system vps-tools; fi
if ! id vps-tools >/dev/null 2>&1; then useradd --system --home-dir /var/lib/vps-tools --shell /usr/sbin/nologin --gid vps-tools vps-tools; fi

target=/opt/vps-tools/releases/$version
[ ! -e "$target" ] || { echo "release already exists: $target" >&2; exit 1; }
tmp="$target.install.$$"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
install -d -m 0755 "$tmp"
install -m 0755 "$release_dir/api" "$tmp/api"
install -m 0755 "$release_dir/runner" "$tmp/runner"
install -m 0755 "$release_dir/backup" "$tmp/backup"
install -m 0755 "$release_dir/vps" "$tmp/vps"
mv "$tmp" "$target"
trap - EXIT HUP INT TERM

install -m 0755 "$script_dir/healthcheck.sh" /usr/local/libexec/vps-tools/healthcheck.sh
install -m 0755 "$script_dir/backup-systemd.sh" /usr/local/libexec/vps-tools/backup-systemd.sh
install -m 0755 "$script_dir/backup-alert.sh" /usr/local/libexec/vps-tools/backup-alert.sh
install -m 0755 "$script_dir/check-backup-freshness.sh" /usr/local/libexec/vps-tools/check-backup-freshness.sh
install -m 0755 "$script_dir/production-acceptance.sh" /usr/local/libexec/vps-tools/production-acceptance.sh
for unit in "$project_dir"/deploy/systemd/*.service "$project_dir"/deploy/systemd/*.timer; do install -m 0644 "$unit" "/etc/systemd/system/$(basename "$unit")"; done
for env in api runner backup; do
    if [ ! -e "/etc/vps-tools/$env.env" ]; then install -m 0600 "$project_dir/deploy/systemd/$env.env.example" "/etc/vps-tools/$env.env"; fi
done
if [ ! -e /etc/vps-tools/known_hosts ]; then install -m 0600 /dev/null /etc/vps-tools/known_hosts; fi
chown -R vps-tools:vps-tools /var/lib/vps-tools
systemctl daemon-reload
systemctl enable vps-tools-backup-freshness.timer

if [ ! -e /opt/vps-tools/current ]; then ln -s "$target" /opt/vps-tools/current; fi
echo "Installed release $version. Edit /etc/vps-tools/api.env and runner.env, then run:"
echo "  systemctl enable --now vps-tools-api.service vps-tools-runner.service"
echo "  systemctl enable --now vps-tools-backup.timer"
echo "  systemctl start vps-tools-backup-freshness.timer"
