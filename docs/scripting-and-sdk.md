# Scripting and SDK guide

Use JSON output or the HTTP API from automation. Do not parse the human-readable tables.

## Shell scripting

Example Bash workflow with strict error handling:

```bash
#!/usr/bin/env bash
set -euo pipefail

api="${VPS_API_URL:-http://localhost:8080}"
user="${VPS_USER:?Set VPS_USER to a provisioned identity}"

health=$(curl --fail-with-body -sS "$api/api/v1/health")
printf '%s\n' "$health"

runbook=$(curl --fail-with-body -sS \
  -H "X-VPS-User: $user" \
  "$api/api/v1/runbooks/check-uptime")
printf '%s\n' "$runbook"

preflight=$(curl --fail-with-body -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "X-VPS-User: $user" \
  "$api/api/v1/runbooks/check-uptime/run" \
  -d '{"target":"server:srv_demo","reason":"scheduled check","params":{},"dry_run":true}')
printf '%s\n' "$preflight"
```

Use `jq` to extract IDs:

```bash
execution_id=$(curl --fail-with-body -sS -X POST \
  -H 'Content-Type: application/json' -H "X-VPS-User: $user" \
  "$api/api/v1/runbooks/check-uptime/run" \
  -d '{"target":"server:srv_demo","reason":"routine check","params":{}}' \
  | jq -r '.execution_id')

curl --fail-with-body -sS -H "X-VPS-User: $user" \
  "$api/api/v1/executions/$execution_id" | jq .
```

Never treat the presence of `execution_id` as success. Poll until the execution is terminal and inspect each target.

## PowerShell scripting

```powershell
$api = $env:VPS_API_URL
if (-not $api) { $api = "http://localhost:8080" }
$headers = @{ "X-VPS-User" = $env:VPS_USER }

$body = @{
  target = "server:srv_demo"
  reason = "routine check"
  params = @{}
  dry_run = $true
} | ConvertTo-Json

$preflight = Invoke-RestMethod -Method Post -Uri "$api/api/v1/runbooks/check-uptime/run" `
  -Headers $headers -ContentType "application/json" -Body $body
$preflight | ConvertTo-Json -Depth 10
```

## Go SDK

The typed SDK is in `packages/sdk-go/client`.

```go
package main

import (
    "fmt"
    "log"

    "github.com/pgd1001/svrtools/packages/sdk-go/client"
)

func main() {
    api := client.New("http://localhost:8080")
    api.SetUser("user_senior")

    runbooks, err := api.SearchRunbooks("uptime")
    if err != nil {
        log.Fatal(err)
    }
    for _, runbook := range runbooks.Runbooks {
        fmt.Println(runbook.Name, runbook.Risk, runbook.Permitted)
    }

    result, err := api.RunRunbook("check-uptime", "server:srv_demo", "routine check", map[string]string{})
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(result["status"], result["execution_id"])
}
```

The SDK also supports server inventory, runners, executions, approvals, audit events, and schedules. See the method names in `packages/sdk-go/client/client.go` for the current typed surface.

## CI patterns

For a read-only CI health check:

```bash
set -euo pipefail
curl --fail-with-body -sS "$VPS_API_URL/api/v1/health" | jq -e '.status == "ok"'
curl --fail-with-body -sS -H "X-VPS-User: $VPS_USER" \
  "$VPS_API_URL/api/v1/servers?environment=production" \
  | jq -e '.servers | type == "array"'
```

For a state-changing CI job, use a dedicated provisioned identity, a named reason, a preflight request, and an external approval gate. Do not put runner credentials or artefact encryption keys in ordinary build logs.

## Idempotency and retries

The current API has execution leases and runner recovery, but scripts should still avoid duplicate submissions. Store the returned execution ID, use a job-specific change identifier in the reason or surrounding system, and check existing execution state before retrying after a network timeout.
