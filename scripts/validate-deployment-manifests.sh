#!/bin/sh
set -eu

# Validate static relationships between deployment manifests. This source-tree
# check does not require Docker, systemd, or Prometheus.

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
systemd_dir="$root/deploy/systemd"
compose_file="$root/deploy/docker-compose/docker-compose.yml"
prometheus_file="$root/deploy/monitoring/prometheus.yml"
alerts_file="$root/deploy/monitoring/vps-tools-alerts.yml"

failures=0
fail() {
    echo "deployment manifest validation: $*" >&2
    failures=$((failures + 1))
}

require_file() {
    [ -f "$1" ] || fail "missing file: ${1#"$root/"}"
}

for file in \
    "$compose_file" "$prometheus_file" "$alerts_file" \
    "$systemd_dir/api.env.example" "$systemd_dir/runner.env.example" "$systemd_dir/backup.env.example" \
    "$systemd_dir/vps-tools-api.service" "$systemd_dir/vps-tools-runner.service" \
    "$systemd_dir/vps-tools-backup.service" "$systemd_dir/vps-tools-backup.timer" \
    "$systemd_dir/vps-tools-backup-freshness.service" "$systemd_dir/vps-tools-backup-freshness.timer" \
    "$systemd_dir/vps-tools-backup-alert.service" "$systemd_dir/vps-tools-healthcheck.service" \
    "$systemd_dir/vps-tools-healthcheck.timer"; do
    require_file "$file"
done

check_contains() {
    file=$1
    pattern=$2
    description=$3
    grep -Eq "$pattern" "$file" || fail "$description (${file#"$root/"})"
}

# Every installed systemd helper referenced by ExecStart must exist in the
# source scripts directory and be included by the installer.
for unit in "$systemd_dir"/*.service; do
    [ -f "$unit" ] || continue
    for command_path in $(sed -n 's/^[[:space:]]*ExecStart[^=]*=[^ ]*\/usr\/local\/libexec\/vps-tools\/\([^[:space:]]*\).*/\/usr\/local\/libexec\/vps-tools\/\1/p' "$unit"); do
        script_name=${command_path##*/}
        require_file "$root/scripts/$script_name"
        check_contains "$root/scripts/install-systemd.sh" "install .*[/\"]$script_name[\" ]" \
            "installer does not install referenced helper $script_name"
    done
done

# Environment files used by services must have release-provided examples.
for env_name in api runner backup; do
    unit="$systemd_dir/vps-tools-$env_name.service"
    if [ -f "$unit" ]; then
        check_contains "$unit" "EnvironmentFile=-/etc/vps-tools/$env_name\.env" \
            "$env_name service has no expected environment file"
        require_file "$systemd_dir/$env_name.env.example"
    fi
done

# Timers must point at a service shipped in the same systemd package.
for timer in "$systemd_dir"/*.timer; do
    [ -f "$timer" ] || continue
    service=$(sed -n 's/^Unit=vps-tools-\(.*\)\.service$/vps-tools-\1.service/p' "$timer")
    [ -n "$service" ] || fail "timer has no service Unit: ${timer##*/}"
    [ -f "$systemd_dir/$service" ] || fail "timer ${timer##*/} references missing service $service"
done

# Keep the local Compose smoke dependencies and monitoring scrape topology
# visible to release checks. YAML syntax remains the responsibility of the
# deployment tool because this repository has no YAML parser dependency here.
check_contains "$compose_file" '^  postgres:' 'Compose manifest has no PostgreSQL service'
check_contains "$compose_file" '^  ssh-target:' 'Compose manifest has no SSH target service'
check_contains "$compose_file" '^volumes:' 'Compose manifest has no persistent volume declaration'
check_contains "$prometheus_file" 'job_name: vps-tools-api' 'Prometheus manifest has no API scrape'
check_contains "$prometheus_file" 'job_name: vps-tools-runner' 'Prometheus manifest has no runner scrape'
check_contains "$prometheus_file" 'vps-tools-alerts\.yml' 'Prometheus manifest has no VPS Tools alert rule file'
check_contains "$alerts_file" 'alert: VPSToolsAPIReadinessFailure' 'alert rules have no readiness alert'
check_contains "$alerts_file" 'alert: VPSToolsDeadLetterJobs' 'alert rules have no dead-letter alert'
check_contains "$alerts_file" 'alert: VPSToolsArtifactStoreLowDisk' 'alert rules have no artifact capacity alert'

[ "$failures" -eq 0 ] || exit 1
echo "deployment manifests validated: systemd, Compose, Prometheus, and alert references"
