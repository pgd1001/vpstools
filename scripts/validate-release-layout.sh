#!/bin/sh
set -eu

dist_dir=${1:-dist}
[ -d "$dist_dir" ] || { echo "release directory not found: $dist_dir" >&2; exit 1; }

archives=$(find "$dist_dir" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \) | sort)
[ -n "$archives" ] || { echo "no release archives found in $dist_dir" >&2; exit 1; }

for archive in $archives; do
    tmp=$(mktemp -d "${TMPDIR:-/tmp}/vps-tools-release.XXXXXX")
    cleanup() { rm -rf -- "$tmp"; }
    trap cleanup EXIT INT TERM
    case "$archive" in
        *.tar.gz) tar -xzf "$archive" -C "$tmp" ;;
        *.zip) command -v unzip >/dev/null 2>&1 || { echo "unzip is required for zip archive validation" >&2; exit 1; }; unzip -q "$archive" -d "$tmp" ;;
    esac

    for file in README.md deploy/README.md \
        scripts/install-systemd.sh scripts/production-acceptance.sh scripts/upgrade-systemd.sh scripts/rollback-systemd.sh \
        scripts/backup-systemd.sh scripts/backup-alert.sh scripts/check-backup-freshness.sh scripts/healthcheck.sh \
        scripts/postgres-backup.sh scripts/postgres-restore.sh \
        migrations/postgres/001_initial_schema.sql migrations/postgres/008_ai_analysis.sql \
        deploy/systemd/api.env.example deploy/systemd/runner.env.example deploy/systemd/backup.env.example \
        deploy/systemd/vps-tools-api.service deploy/systemd/vps-tools-runner.service \
        deploy/systemd/vps-tools-backup.service deploy/systemd/vps-tools-backup.timer \
        deploy/systemd/vps-tools-backup-alert.service deploy/systemd/vps-tools-healthcheck.service \
        deploy/systemd/vps-tools-healthcheck.timer \
        deploy/systemd/vps-tools-backup-freshness.service deploy/systemd/vps-tools-backup-freshness.timer; do
        [ -e "$tmp/$file" ] || { echo "$(basename "$archive") is missing $file" >&2; exit 1; }
    done

    archive_lower=$(basename "$archive" | tr '[:upper:]' '[:lower:]')
    if printf '%s' "$archive_lower" | grep -q '_windows_'; then
        for binary in api.exe runner.exe backup.exe vps.exe; do
            [ -f "$tmp/$binary" ] || { echo "$(basename "$archive") is missing $binary" >&2; exit 1; }
        done
    else
        for binary in api runner backup vps; do
            [ -x "$tmp/$binary" ] || { echo "$(basename "$archive") has a non-executable or missing binary: $binary" >&2; exit 1; }
        done
    fi
    cleanup
    trap - EXIT INT TERM
    echo "release archive layout verified: $archive"
done
