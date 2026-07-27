#!/bin/sh
set -eu

# Run the final self-contained deployment checks on the host that will run
# VPS Tools. This script never prints tokens or authenticated API responses.

api_url=${VPS_API_URL:-http://127.0.0.1:${API_PORT:-8080}}
runner_health_url=${RUNNER_HEALTH_URL:-http://127.0.0.1:9091/health}
runner_metrics_url=${RUNNER_METRICS_URL:-http://127.0.0.1:9091/metrics}
api_metrics_url=${API_METRICS_URL:-${api_url%/}/metrics}
cli_binary=${CLI_BINARY:-/opt/vps-tools/current/vps}
backup_status_file=${BACKUP_STATUS_FILE:-/var/lib/vps-tools/backups/backup-status.json}
backup_check=${BACKUP_CHECK:-/usr/local/libexec/vps-tools/check-backup-freshness.sh}
report_file=${PRODUCTION_EVIDENCE_FILE:-}
skip_systemd=${SKIP_SYSTEMD_CHECK:-false}
failures=0
started_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
results=$(mktemp "${TMPDIR:-/tmp}/vps-tools-acceptance.XXXXXX")
trap 'rm -f "$results"' EXIT HUP INT TERM

record() {
    status=$1
    name=$2
    detail=$3
    printf '%s\t%s\t%s\n' "$status" "$name" "$detail" >> "$results"
}

pass() {
    record pass "$1" "$2"
    printf '[PASS] %s, %s\n' "$1" "$2"
}

fail() {
    record fail "$1" "$2"
    failures=$((failures + 1))
    printf '[FAIL] %s, %s\n' "$1" "$2" >&2
}

check_command() {
    name=$1
    detail=$2
    shift 2
    if "$@" >/dev/null 2>&1; then
        pass "$name" "$detail"
    else
        fail "$name" "$detail"
    fi
}

check_curl() {
    name=$1
    url=$2
    detail=$3
    if curl --fail --silent --show-error --max-time 10 "$url" >/dev/null 2>&1; then
        pass "$name" "$detail"
    else
        fail "$name" "$detail"
    fi
}

printf 'VPS Tools production acceptance\n'
printf 'API: %s\n' "$api_url"
printf 'Started: %s\n\n' "$started_at"

if [ "${VPS_ENV:-}" = production ]; then
    pass "production environment" "VPS_ENV=production"
else
    fail "production environment" "set VPS_ENV=production before accepting a deployment"
fi

if command -v curl >/dev/null 2>&1; then
    pass "curl available" "HTTP checks can run"
else
    fail "curl available" "curl is required for host acceptance"
fi

if [ -x "$cli_binary" ] || command -v "$cli_binary" >/dev/null 2>&1; then
    if VPS_API_URL="$api_url" "$cli_binary" doctor --json >/dev/null 2>&1; then
        pass "authenticated doctor" "API health, readiness, and identity passed"
    else
        fail "authenticated doctor" "vps doctor could not verify health, readiness, and identity"
    fi
else
    fail "authenticated doctor" "CLI_BINARY is missing or not executable"
fi

check_curl "API readiness" "${api_url%/}/api/v1/ready" "database and encrypted artefact store are ready"
check_curl "API metrics" "$api_metrics_url" "control-plane metrics endpoint responds"
check_curl "runner health" "$runner_health_url" "runner health endpoint responds"
check_curl "runner metrics" "$runner_metrics_url" "runner metrics endpoint responds"

if [ "$skip_systemd" = true ]; then
    pass "systemd services" "skipped by SKIP_SYSTEMD_CHECK=true"
elif command -v systemctl >/dev/null 2>&1; then
    check_command "API service" "vps-tools-api.service is active" systemctl is-active --quiet vps-tools-api.service
    check_command "runner service" "vps-tools-runner.service is active" systemctl is-active --quiet vps-tools-runner.service
else
    fail "systemd services" "systemctl is unavailable, set SKIP_SYSTEMD_CHECK=true only for a non-systemd deployment"
fi

if [ -x "$backup_check" ]; then
    if BACKUP_STATUS_FILE="$backup_status_file" "$backup_check" >/dev/null 2>&1; then
        pass "backup freshness" "latest backup is verified and within the configured age"
    else
        fail "backup freshness" "backup status, manifest, or verification failed"
    fi
else
    fail "backup freshness" "BACKUP_CHECK is missing or not executable"
fi

finished_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
if [ -n "$report_file" ]; then
    report_dir=$(dirname "$report_file")
    mkdir -p "$report_dir"
    tab=$(printf '\t')
    {
        printf '# VPS Tools production acceptance\n\n'
        printf -- '- Started, %s\n' "$started_at"
        printf -- '- Finished, %s\n' "$finished_at"
        printf -- '- API endpoint, %s\n' "$api_url"
        printf -- '- Token values and authenticated response bodies, omitted\n\n'
        printf '| Check | Status | Detail |\n|---|---|---|\n'
        while IFS="$tab" read -r status name detail; do
            [ -n "$status" ] || continue
            printf '| %s | %s | %s |\n' "$name" "$status" "$detail"
        done < "$results"
        printf '\nResult, %s\n' "$( [ "$failures" -eq 0 ] && printf 'PASS' || printf 'FAIL' )"
    } > "$report_file"
    printf '\nEvidence report: %s\n' "$report_file"
fi

if [ "$failures" -ne 0 ]; then
    printf '\nProduction acceptance failed with %s check(s).\n' "$failures" >&2
    exit 1
fi

printf '\nProduction acceptance passed.\n'
