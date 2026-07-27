#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
api_bin=${API_BINARY:-$project_root/bin/api}
runner_bin=${RUNNER_BINARY:-$project_root/bin/runner}
backup_bin=${BACKUP_BINARY:-$project_root/bin/backup}
cli_bin=${CLI_BINARY:-$project_root/bin/vps}
port=${API_PORT:-18080}

for binary in "$api_bin" "$runner_bin" "$backup_bin" "$cli_bin"; do
    [ -x "$binary" ] || { echo "required executable is missing: $binary" >&2; exit 2; }
done
command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 2; }

root=$(mktemp -d "${TMPDIR:-/tmp}/vps-tools-smoke.XXXXXX")
api_pid=
runner_pid=
restored_api_pid=
cleanup() {
    [ -z "$runner_pid" ] || kill "$runner_pid" 2>/dev/null || true
    [ -z "$api_pid" ] || kill "$api_pid" 2>/dev/null || true
    [ -z "$restored_api_pid" ] || kill "$restored_api_pid" 2>/dev/null || true
    wait "$runner_pid" 2>/dev/null || true
    wait "$api_pid" 2>/dev/null || true
    wait "$restored_api_pid" 2>/dev/null || true
    rm -rf -- "$root"
}
trap cleanup EXIT INT TERM

export DATABASE_URL="$root/svrtools.db"
export VPS_ARTIFACTS_DIR="$root/artifacts"
export ARTIFACTS_DIR="$VPS_ARTIFACTS_DIR"
export API_PORT="$port"
export VPS_ENV=development
export VPS_DEV_AUTH=true
export VPS_API_URL="http://127.0.0.1:$port"
export VPS_USER=user_senior
export VPS_API_TOKEN=
export BACKUP_ENCRYPTION_KEY=YmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmJiYmI=

"$api_bin" >"$root/api.out" 2>"$root/api.err" &
api_pid=$!
ready_response=
for _ in $(seq 1 60); do
    if ready_response=$(curl -fsS "$VPS_API_URL/api/v1/ready" 2>/dev/null); then break; fi
    sleep 0.25
done
printf '%s' "$ready_response" | grep -q '"artifacts":"ok"' || { cat "$root/api.err" >&2; echo "API did not report artifact readiness" >&2; exit 1; }

metrics=$(curl -fsS "$VPS_API_URL/metrics")
printf '%s\n' "$metrics" | grep -q 'svrtools_api_requests_total'

token_response=$(curl -fsS -X POST "$VPS_API_URL/api/v1/auth/tokens" \
    -H 'X-VPS-User: user_senior' -H 'Content-Type: application/json' \
    -d '{"name":"self-contained-smoke","user_id":"user_senior","expires_in":3600}')
api_token=$(printf '%s' "$token_response" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
[ -n "$api_token" ] || { echo "API token issuance failed" >&2; exit 1; }
export VPS_API_TOKEN="$api_token"
export VPS_USER=
identity=$(curl -fsS "$VPS_API_URL/api/v1/whoami" -H "Authorization: Bearer $VPS_API_TOKEN")
printf '%s\n' "$identity" | grep -q 'user_senior'
"$cli_bin" doctor --api-url "$VPS_API_URL" >/dev/null

pause=$(curl -fsS -X POST "$VPS_API_URL/api/v1/automation/pause" \
    -H "Authorization: Bearer $VPS_API_TOKEN" -H 'Content-Type: application/json' \
    -d '{"reason":"self-contained smoke test"}')
printf '%s\n' "$pause" | grep -q '"paused":true'
resume=$(curl -fsS -X POST "$VPS_API_URL/api/v1/automation/resume" -H "Authorization: Bearer $VPS_API_TOKEN")
printf '%s\n' "$resume" | grep -q '"paused":false'

schedule=$(curl -fsS -X POST "$VPS_API_URL/api/v1/schedules" \
    -H "Authorization: Bearer $VPS_API_TOKEN" -H 'Content-Type: application/json' \
    -d '{"name":"self-contained-smoke-schedule","runbook_name":"check-uptime","target":"server:srv_demo","reason":"self-contained smoke test","params":{},"interval_seconds":3600}')
printf '%s\n' "$schedule" | grep -q '"status":"created"'
schedules=$(curl -fsS "$VPS_API_URL/api/v1/schedules" -H "Authorization: Bearer $VPS_API_TOKEN")
schedule_id=$(printf '%s' "$schedules" | sed -n 's/.*"id":"\(sch_[^"]*\)".*"name":"self-contained-smoke-schedule".*/\1/p')
[ -n "$schedule_id" ] || { echo "schedule listing failed" >&2; exit 1; }
disabled=$(curl -fsS -X DELETE "$VPS_API_URL/api/v1/schedules/$schedule_id" -H "Authorization: Bearer $VPS_API_TOKEN")
printf '%s\n' "$disabled" | grep -q '"status":"disabled"'

export API_URL="$VPS_API_URL"
export SIMULATE=true
runner_health_port=$((port + 2))
export RUNNER_HEALTH_ADDR="127.0.0.1:$runner_health_port"
runner_token_response=$(curl -fsS -X POST "$VPS_API_URL/api/v1/runners/registration-token" -H 'X-VPS-User: user_senior')
export VPS_RUNNER_TOKEN=$(printf '%s' "$runner_token_response" | sed -n 's/.*"registration_token":"\([^"]*\)".*/\1/p')
[ -n "$VPS_RUNNER_TOKEN" ] || { echo "runner token issuance failed" >&2; exit 1; }
RUNNER_NAME=self-contained-smoke-runner "$runner_bin" >"$root/runner.out" 2>"$root/runner.err" &
runner_pid=$!
runner_health_response=
for _ in $(seq 1 60); do
    if runner_health_response=$(curl -fsS "http://127.0.0.1:$runner_health_port/health" 2>/dev/null); then break; fi
    sleep 0.25
done
printf '%s' "$runner_health_response" | grep -q '"status":"healthy"' || { cat "$root/runner.err" >&2; echo "runner health endpoint did not become ready" >&2; exit 1; }

"$cli_bin" exec server:srv_demo --reason 'self-contained smoke test' --wait --timeout 30 -- uptime

backup_dir="$root/backups/latest"
replica_dir="$root/replica/latest"
restore_db="$root/restore/svrtools.db"
restore_artifacts="$root/restore/artifacts"
mkdir -p "$(dirname "$backup_dir")" "$(dirname "$replica_dir")"
"$backup_bin" -db "$DATABASE_URL" -artifacts "$VPS_ARTIFACTS_DIR" -output "$backup_dir"
"$backup_bin" -mode verify -input "$backup_dir"
cp -a -- "$backup_dir" "$replica_dir"
"$backup_bin" -mode verify -input "$replica_dir"
"$backup_bin" -mode restore -input "$replica_dir" -db "$restore_db" -artifacts "$restore_artifacts"
[ -f "$restore_db" ] && [ -d "$restore_artifacts" ]
restored_port=$((port + 1))
DATABASE_URL="$restore_db" VPS_ARTIFACTS_DIR="$restore_artifacts" ARTIFACTS_DIR="$restore_artifacts" API_PORT="$restored_port" VPS_ENV=development VPS_DEV_AUTH=true \
    "$api_bin" >"$root/restored-api.out" 2>"$root/restored-api.err" &
restored_api_pid=$!
restored_ready_response=
for _ in $(seq 1 60); do
    if restored_ready_response=$(curl -fsS "http://127.0.0.1:$restored_port/api/v1/ready" 2>/dev/null); then break; fi
    sleep 0.25
done
printf '%s' "$restored_ready_response" | grep -q '"artifacts":"ok"' || { cat "$root/restored-api.err" >&2; echo "restored API did not report artifact readiness" >&2; exit 1; }
curl -fsS "http://127.0.0.1:$restored_port/api/v1/whoami" -H 'X-VPS-User: user_senior' | grep -q 'user_senior'
curl -fsS "http://127.0.0.1:$restored_port/api/v1/executions" -H 'X-VPS-User: user_senior' | grep -q 'execution'
curl -fsS "http://127.0.0.1:$restored_port/api/v1/audit" -H 'X-VPS-User: user_senior' | grep -q 'events'
curl -fsS "http://127.0.0.1:$restored_port/api/v1/audit/verify" -H 'X-VPS-User: user_senior' | grep -q '"valid":true'
echo "self-contained smoke and backup restore passed. Temporary state was removed."
