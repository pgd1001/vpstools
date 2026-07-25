# Developer Guide

## Prerequisites

- **Go 1.24+**
- **Docker** (optional — only needed for PostgreSQL and SSH test target)

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
      # Use ${PARAM_NAME:-default} for parameter substitution
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
- `DB_PATH` env var (defaults to `svrtools.db`)

### Runner
- `API_URL` env var (defaults to `http://localhost:8080`)
- `SIMULATE=true` to skip SSH and fake execution
- `SSH_TARGET_HOST`, `SSH_TARGET_PORT`, `SSH_USER`, `SSH_PASSWORD` for real SSH
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
