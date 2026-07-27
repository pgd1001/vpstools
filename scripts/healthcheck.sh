#!/bin/sh
set -eu

api_only=false
if [ "${1:-}" = "--api-only" ]; then
    api_only=true
fi

api_url=${API_URL:-${VPS_API_URL:-http://127.0.0.1:${API_PORT:-8080}}}
health_url="${api_url%/}/api/v1/ready"

if ! command -v curl >/dev/null 2>&1; then
    echo "health check failed: curl is required" >&2
    exit 1
fi

curl --fail --silent --show-error --max-time 10 "$health_url" >/dev/null

runner_health_url=${RUNNER_HEALTH_URL:-http://127.0.0.1:9091/health}
if [ "$api_only" = false ]; then
    curl --fail --silent --show-error --max-time 10 "$runner_health_url" >/dev/null
fi

if [ "$api_only" = false ] && command -v systemctl >/dev/null 2>&1; then
    systemctl is-active --quiet vps-tools-api.service
    systemctl is-active --quiet vps-tools-runner.service
fi

echo "vps-tools readiness check passed"
