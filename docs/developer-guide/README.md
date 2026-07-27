# Developer Guide

The [documentation hub](../README.md) links to the user, operator, API, automation, AI, security, and migration guides. This page focuses on the repository, local development, interfaces, and extension points.

## Prerequisites

- **Go version from `go.mod`**
- **Docker** (optional, only needed for PostgreSQL and SSH test target)

## Quick start (self-contained, no Docker)

```powershell
# Build all binaries
go build -o bin/vps.exe ./apps/cli
go build -o bin/api.exe ./apps/api
go build -o bin/runner.exe ./apps/runner

# Terminal 1: Start API (auto-migrates + seeds SQLite)
.\bin\api.exe

# Terminal 2: Queue an execution
.\bin\vps.exe exec server:demo -- uptime

# Terminal 3: Run the runner (simulate mode)
$env:VPS_DEV_AUTH = "true"
$env:SIMULATE = "true"
.\bin\runner.exe

# Terminal 2: Check audit trail
.\bin\vps.exe audit search --limit 5
```

## With Docker (PostgreSQL + real SSH target)

```bash
# Start dependencies
docker compose -f deploy/docker-compose/docker-compose.yml up -d

# Build
make api cli runner

# Run
.\bin\api.exe
.\bin\vps.exe exec server:demo -- uptime
.\bin\runner.exe
```

## Dev commands

```bash
make test       # go test ./...
make vet        # go vet ./...
make generate   # buf generate (protobuf) + sqlc generate (queries)
make lint       # golangci-lint run ./...
make build      # Build CLI + API + runner
```

## Architecture

```
vps exec → HTTP POST → API → SQLite → runner polls GET /api/v1/jobs/next
  → executes (real SSH or simulate) → POST /api/v1/jobs/result → API
  → execution result + audit event
```

## API endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/v1/health` | GET | Health check |
| `/api/v1/whoami` | GET | Current user/org/role |
| `/api/v1/servers` | GET | List servers |
| `/api/v1/executions` | POST | Create execution |
| `/api/v1/jobs/next` | GET | Poll next job (runner) |
| `/api/v1/jobs/result` | POST | Submit result (runner) |
| `/api/v1/runbooks` | GET | List runbooks (supports `?search=` filter) |
| `/api/v1/runbooks` | POST | Create runbook |
| `/api/v1/runbooks/:name` | GET | Get runbook detail |
| `/api/v1/runbooks/:name/publish` | POST | Publish runbook |
| `/api/v1/runbooks/:name/run` | POST | Execute runbook |
| `/api/v1/schedules` | GET, POST | List or create interval schedules |
| `/api/v1/schedules/:id` | DELETE | Disable a schedule |
| `/api/v1/audit` | GET | Search audit events |

## Runbook Templates

41 runbook YAML templates live in `runbooks/` and are validated by `runbooks/validate_runbooks_test.go`:

| Wave | Count | Risk | Description |
|---|---|---|---|
| Examples | 4 | low | check-disk, check-memory, restart-nginx, tail-logs |
| Diagnostics | 7 | low | system-info, network-diag, process-top, ssl-cert-check, docker-stats, failed-auth-report, journal-check |
| Provisioning | 7 | high | base-hardened-ubuntu, docker-server, dokploy-install, nextcloud-aio, seafile-install, hermes-agent, ai-code-tools |
| AI Stack | 6 | medium | ollama-openwebui-opencode, n8n-ai-starter-kit, selfhosted-ai-package, agixt-platform, paperclip-ai, ezlocal-ai |
| Maintenance | 6 | medium | system-update, docker-cleanup, log-cleanup, config-backup, cert-renew, service-rotate |
| Security | 7 | low | audit-ports, user-audit, fail2ban-status, ufw-status, disk-usage-deep, io-stat, memory-report |
| Recovery | 4 | high | service-restart, docker-restart, swap-manage, emergency-cleanup |

Runbook YAML structure:

```yaml
apiVersion: vps-tools.io/v1
kind: Runbook
metadata:
  name: unique-name
  title: Human Readable Title
  risk: low|medium|high
  tags: ["category", "sub-category"]
spec:
  parameters:
    - name: param_name
      type: string
      default: "value"
  execution:
    command: |
      #!/bin/bash
      # Define parameters explicitly and use ${PARAM_NAME} for substitutions.
      # Shell defaults such as ${PARAM_NAME:-default} are not runbook inputs.
  approval:
    required: true
    requiredRoles: ["admin", "senior_engineer"]
    environment: production
```

## Configuration

### CLI
- `--api-url` flag or `VPS_API_URL` env var (defaults to `http://localhost:8080`)
- Config file at `~/.config/vps-tools/config.yaml` (optional)

### API
- `API_PORT` env var (defaults to `8080`)
- `DATABASE_DRIVER` and `DATABASE_URL` select the metadata database. The default is SQLite at `./svrtools.db`.
- `ARTIFACT_STORE` and `ARTIFACTS_DIR` select encrypted output storage. The default is local storage at `./data/artifacts`.
- When `ARTIFACT_STORE=s3`, set `S3_ENDPOINT`, `S3_BUCKET`, and, when required by the service, `S3_ACCESS_KEY_ID` and `S3_SECRET_ACCESS_KEY`. `S3_REGION`, `S3_PREFIX`, `S3_ENCRYPTION_KEY`, `S3_SERVER_SIDE_ENCRYPTION`, `S3_SSE_KMS_KEY_ID`, `S3_TIMEOUT`, `S3_MAX_RETRIES`, and `S3_RETRY_BACKOFF` are optional tuning and protection settings. The artifact store can issue seven-day-or-shorter SigV4 read URLs when access keys are configured.
- `JOB_DISPATCH`, `SCHEDULER`, and `EVENT_BUS` select queue, scheduler, and event settings. Defaults are `database`, `embedded`, and `disabled`.
- `ARTIFACT_ENCRYPTION_KEY` can provide a base64-encoded 32-byte key. If omitted, the local store creates `ARTIFACTS_DIR/.key`.

### Runner
- `API_URL` env var (defaults to `http://localhost:8080`)
- `SIMULATE=true` to skip SSH and fake execution
- `SSH_TARGET_HOST`, `SSH_TARGET_PORT`, `SSH_USER`, `SSH_PASSWORD` for real SSH

### Deployment backends

The self-contained tier is the supported default and requires no external services. It uses SQLite WAL mode, encrypted local artefacts, database polling, and the embedded scheduler.

The configuration loader recognises PostgreSQL, S3-compatible storage, JetStream, external scheduling, and NATS event settings. It validates required connection variables and reports the selected tier at API startup. The request handlers use the shared SQL runtime for SQLite and PostgreSQL, the API applies and verifies the versioned PostgreSQL migrations, can compose the S3 artefact store, and can use JetStream as a database-authoritative runner notification bridge when its complete configuration is supplied. Row-level security, external scheduling, NATS events, full independent queue dispatch, complete object-store backup and restore, and horizontally scaled workers remain tracked limitations. Unsupported selections fail at startup rather than silently falling back to local services.

### Automation and AI boundaries

Schedules use a fixed interval and the same runbook policy checks as manual execution. Each queued scheduled execution records `user_automation` as its actor and includes a schedule reference in the audit metadata. High and critical risk schedules are rejected from unattended execution.

The `packages/ai` package defines a provider interface, a redacting wrapper for prompts, evidence, and responses, and an OpenAI-compatible HTTP provider for managed gateways or local model servers. When `AI_PROVIDER=openai-compatible`, the API exposes a bounded read-only analysis endpoint. It can use supplied evidence or redacted output from an execution, persists request metadata and evidence in SQLite, and writes an audit event. The same operation is available through `vps ai analyze`, the Go SDK, the web execution detail view, and `vps_analyse_read_only` in MCP. Retrieval across an organisation, conversations, streaming, provider failover, and model administration remain future work.

### MCP and agent integration

The `mcp/` package provides a local stdio MCP server for compatible AI clients. It exposes identity, health, inventory, runbook discovery, preflight, approvals, execution monitoring, schedules, and audit search.

Read tools are enabled by default. State-changing tools require both `VPS_MCP_ALLOW_WRITES=true` and a tool-level `confirm=true` supplied only after explicit user confirmation. The MCP server does not expose arbitrary shell execution.

See [the MCP setup guide](../../mcp/README.md) and [the VPS Tools agent skill](../../skills/vpstools-operations/SKILL.md) for configuration and operating rules.
# Self-contained deployment

The default API deployment requires no PostgreSQL, object storage, or message broker.
It uses SQLite in WAL mode, an encrypted local artefact directory, and database-backed job polling.

```text
DATABASE_DRIVER=sqlite
DATABASE_URL=./svrtools.db
ARTIFACT_STORE=local
ARTIFACTS_DIR=./data/artifacts
JOB_DISPATCH=database
SCHEDULER=embedded
EVENT_BUS=disabled
```

The local artefact store creates an encryption key at `ARTIFACTS_DIR/.key` on first start.
Back up this file with the database. Losing it makes encrypted artefacts unrecoverable.

Create a consistent backup with:

```bash
make backup
```

Large execution output is written as encrypted artefacts and referenced from SQLite.
Small output remains inline for fast reads.

## Extended deployment

Larger installations can select PostgreSQL, S3-compatible storage, and NATS through the same backend settings:

```text
DATABASE_DRIVER=postgres
DATABASE_URL=postgres://...
ARTIFACT_STORE=s3
S3_ENDPOINT=https://...
JOB_DISPATCH=jetstream
NATS_URL=nats://...
SCHEDULER=external
EVENT_BUS=nats
```

The API validates these settings at startup and refuses incomplete configurations. The self-contained SQLite path remains the supported local and small-deployment path.
