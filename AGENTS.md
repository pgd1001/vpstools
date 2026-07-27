# AGENTS.md

## Current state

**Phase 0 (Architecture Spike) is implemented.** The CLI→API→Runner→audit vertical slice works. No Docker or PostgreSQL required for local dev — the spike uses pure-Go SQLite and a simulate mode for the runner.

**Phase 7 (Hardening and Beta) is implemented.** Secret redaction, runner scope checks, 10 security tests (RBAC, audit completeness, tenant isolation, cross-org job claims), known limitations doc, operator guide, GoReleaser config.

**Post-Phase-7 additions:** 24 maintenance runbook templates (diagnostics, maintenance, security, recovery) — 41 runbooks total. Runbook search via TUI (`/` key filter) and API (`?search=` query param).

**Go module:** `github.com/pgd1001/svrtools` (Go 1.26.5)
**Build output:** `bin/vps.exe`, `bin/api.exe`, `bin/runner.exe`

The three planning files remain authoritative for future phases:
- `vps_tools_prd.md` — product requirements and scope
- `vps_tools_technical_specification.md` — stack, architecture, API design
- `vps_tools_mvp_build_plan.md` — phased MVP build order

## Planned stack (from technical spec)

- **CLI + API + Runner:** Go (monorepo)
- **CLI framework:** Cobra + Charm (Bubble Tea, Bubbles, Huh, Lip Gloss, Glamour)
- **API contracts:** ConnectRPC + protobuf
- **Database:** PostgreSQL with `sqlc` for type-safe queries, Goose/Atlas for migrations
- **Queue/events:** NATS JetStream
- **Object storage:** S3-compatible (MinIO for local/self-hosted)
- **Authorization:** OpenFGA for relationship-based access
- **Policy engine:** Application-level YAML policy evaluator (MVP); OPA post-MVP
- **Identity:** OIDC-first; Zitadel/Keycloak for self-hosted
- **Web console:** Next.js + TypeScript + Tailwind CSS
- **Telemetry:** OpenTelemetry, Prometheus metrics, structured JSON logs
- **Build/release:** GoReleaser, Docker, Docker Compose, Syft, Cosign, Trivy

## Planned monorepo layout (not yet created)

```
apps/
  cli/          # Go CLI/TUI binary (vps)
  api/          # Go control plane API
  runner/       # Go execution runner
  web/          # Next.js web console
packages/
  proto/        # Protocol Buffer schemas
  sdk-go/       # Generated Go SDK helpers
  sdk-ts/       # Generated TypeScript SDK
  authz/        # Shared authorization models
  audit/        # Shared audit event definitions
  runbooks/     # Runbook schema and validator
  sshx/         # SSH execution/session helpers
deploy/
migrations/
  postgres/
```

## Spike architecture (what's actually built)

```
apps/
  cli/          # Cobra CLI (main.go, root.go, whoami.go, server.go, exec.go, audit.go)
  api/          # net/http API + SQLite (main.go, helpers.go, migrate/seed stubs)
  runner/       # SSH executor with SIMULATE fallback
packages/
  sdk-go/client/  # HTTP client for CLI→API communication
  sshx/           # Go crypto/ssh command execution (error handling tested)
  audit/          # Audit event Go struct
  runbooks/       # Runbook YAML schema types
  proto/          # Protobuf definitions (not yet code-generated — Phase 1)
deploy/docker-compose/  # Docker Compose + SSH target Dockerfile (Docker required)
migrations/postgres/    # PostgreSQL migration (not yet applied — Phase 1)
```

**Current data flow:** `vps exec` → HTTP POST to API → SQLite insert → runner polls `GET /api/v1/jobs/next` → executes (real SSH or simulate) → `POST /api/v1/jobs/result` → API writes execution result + audit event.

**API endpoints (Phase 0):**
| Endpoint | Method | Purpose |
|---|---|---|
| `/api/v1/health` | GET | Health check |
| `/api/v1/whoami` | GET | Current user/org/role |
| `/api/v1/servers` | GET | List servers |
| `/api/v1/executions` | POST | Create execution |
| `/api/v1/jobs/next` | GET | Poll next job (runner) |
| `/api/v1/jobs/result` | POST | Submit result (runner) |
| `/api/v1/audit` | GET | Search audit events |

## Dev commands (Phase 0 — self-contained)

```bash
go build -o bin/vps.exe ./apps/cli      # Build CLI
go build -o bin/api.exe ./apps/api      # Build API server
go build -o bin/runner.exe ./apps/runner # Build runner
go test ./...                            # Run all tests
go vet ./...                             # Vet all packages
make build                               # Build all three binaries
make test                                # Run tests
make lint                                # golangci-lint
make generate                            # buf generate + sqlc generate
```

**No Docker or PostgreSQL required for local dev.** The API uses embedded SQLite (`svrtools.db`) and auto-migrates/auto-seeds on startup. The runner supports `SIMULATE=true` mode to skip SSH.

**With PostgreSQL:** Set `DATABASE_URL` and run `go run apps/api/cmd/migrate/main.go -- up` for Goose migrations. PostgreSQL required for production target; SQLite for local dev spike.

### Run the full vertical slice

```bash
# Terminal 1: Start API (auto-migrates + seeds on first run)
.\bin\api.exe

# Terminal 2: Queue an execution
.\bin\vps.exe exec server:demo -- uptime

# Terminal 3: Run the runner (simulate mode — no SSH needed)
$env:SIMULATE = "true"
.\bin\runner.exe

# Terminal 2: Check audit trail
.\bin\vps.exe audit search --limit 5
```

## Key architectural invariants (from the planning docs)

- **Control plane is the single policy enforcement point** — never trust the CLI or runner to authorize independently.
- **Runner must not make access decisions** — it only executes signed jobs that the API authorized.
- **Deny-by-default** for all execution — permissions must be explicitly granted.
- **Every privileged action must create an immutable audit event** in the append-only audit table.
- **No shared user accounts** — every actor has a distinct identity.
- **No secrets in logs, audit command previews, or CLI debug output.**
- **Tenant isolation** — every resource is scoped to an organization; cross-org access must be impossible.
- **Runbooks are versioned and immutable once published** — executions reference exact versions.

## Build order (from MVP plan)

Phases are intended to be sequential vertical slices (each phase builds a working feature, not a layer):

1. **Phase 0 — Architecture Spike:** ~~Prove CLI→API→Runner→SSH→audit path~~ **DONE**
2. **Phase 1 — Foundations:** ~~Monorepo, CI, migrations, code generation~~ **DONE**
3. **Phase 2 — Inventory and Runner:** ~~Server registration, runner heartbeat, health checks~~ **DONE**
4. **Phase 3 — Execution Engine:** ~~Single/group execution, output capture, timeouts~~ **DONE**
5. **Phase 4 — RBAC and Policy:** ~~Role enforcement, policy evaluation, deny-by-default~~ **DONE**
6. **Phase 5 — Runbooks and Approvals:** ~~Delegated runbook creation and approval workflow~~ **DONE**
7. **Phase 6 — TUI and Web Console:** ~~Interactive operator UX~~ **DONE**
8. **Phase 7 — Hardening and Beta:** ~~Security review, redaction, docs, packaging~~ **DONE**

## Security gates before private beta (reference only)

These must pass before any beta release: authorization correctness, runner trust boundary, audit completeness, secret safety, and tenant isolation. See `vps_tools_mvp_build_plan.md` sections 19-21 for full criteria.
