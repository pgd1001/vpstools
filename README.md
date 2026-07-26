# VPS Tools

VPS Tools is a controlled operations platform for infrastructure teams. Senior engineers define and publish runbooks. Junior engineers complete permitted tasks through guided forms, the CLI, or the TUI. Approvals, execution state, audit events, automation, and evidence stay connected to the same runbook version.

## What is included

- Versioned runbooks with parameter validation and shell-safe rendering
- Role-based access for senior, junior, approver, administrator, and auditor workflows
- Approval requests with reasons, expiry, target snapshots, and audit history
- Database-backed execution with runner leases, retries, output redaction, and timelines
- SQLite and encrypted local artefacts as the default self-contained deployment
- Configuration points for larger PostgreSQL, S3-compatible, and NATS deployments
- Embedded interval schedules with an explicit automation identity and conservative risk rules
- CLI, Bubble Tea TUI, and web console access to the core workflow
- A vendor-neutral AI provider boundary with evidence and redaction contracts

## Quick start

The default installation needs Go only. PostgreSQL, S3, NATS, and Docker are not required.

```powershell
go build -o bin/vps.exe ./apps/cli
go build -o bin/api.exe ./apps/api
go build -o bin/runner.exe ./apps/runner

# Terminal 1
.\bin\api.exe

# Terminal 2
.\bin\vps.exe exec server:demo -- uptime

# Terminal 3
$env:SIMULATE = "true"
.\bin\runner.exe

# Terminal 2
.\bin\vps.exe audit search --limit 5
```

The API auto-migrates and seeds a local SQLite database. The runner can use simulated execution for development or trusted SSH for real targets.

## Deployment tiers

The self-contained tier is the default.

```text
DATABASE_DRIVER=sqlite
DATABASE_URL=./svrtools.db
ARTIFACT_STORE=local
ARTIFACTS_DIR=./data/artifacts
JOB_DISPATCH=database
SCHEDULER=embedded
EVENT_BUS=disabled
```

SQLite runs in WAL mode with a single-writer-safe connection limit. Artefacts are encrypted locally, written atomically, and referenced by stable IDs. Use `make backup` to include database metadata, artefact files, and the manifest.

Larger deployments can select the extended configuration shape below. Incomplete external settings fail at startup rather than silently falling back.

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

The current API runtime is fully implemented for the self-contained tier. External adapter work is tracked in the [known limitations](docs/KNOWN_LIMITATIONS.md).

## Operator workflow

1. A senior engineer creates and publishes a runbook.
2. A junior engineer selects a permitted task and runs preflight checks.
3. The API validates the target, environment, parameters, risk, and approval policy.
4. The task is queued directly or sent for approval.
5. A runner claims the work, executes it, and submits redacted results.
6. Operators follow the execution timeline and audit trail.

Interval schedules are available to senior engineers through the API and web console. High and critical risk runbooks are not executed unattended. They require an approval-backed workflow, which remains planned.

## Interfaces

- CLI commands are documented in [the operator guide](docs/operator-guide/README.md).
- The TUI is available with `vps tui`. Use keys `1` through `6` for servers, runbooks, executions, approvals, schedules, and audit.
- Start the web console from `apps/web` with `npm install` followed by `npm run dev`.
- API and SDK details are in [the developer guide](docs/developer-guide/README.md).

## Development checks

```bash
go test ./...
go vet ./...
make build
cd apps/web && npm run build
```

## Documentation

- [Product improvement plan](docs/PRODUCT_IMPROVEMENT_PLAN.md)
- [Operator guide](docs/operator-guide/README.md)
- [Developer guide](docs/developer-guide/README.md)
- [Runbook catalog](docs/runbooks/README.md)
- [Known limitations](docs/KNOWN_LIMITATIONS.md)
- [OIDC deployment guide](docs/operator-guide/OIDC_ZITADEL.md)
