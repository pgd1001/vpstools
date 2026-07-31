# VPS Tools: Known Limitations (MVP)

## Authentication & Identity

- **Development auth is header-based.** Set `VPS_DEV_AUTH=true` only for local demos. Production web access supports the configured OIDC session flow, and CLI, SDK, and automation clients can use expiring API bearer tokens. A direct CLI OIDC login flow remains future work.
- **API tokens are operator-managed.** Tokens are stored as hashes, expire, can be revoked, and are shown once. A secret manager integration and token self-service UI remain future work.

## Execution & Runner

- **Simulated SSH when Docker unavailable.** Set `SIMULATE=true` for local dev without real SSH target.
- **SSH credentials are per server and held only by the runner.** Each server records a credential reference and the SHA256 fingerprint of its host key. The runner resolves the reference against its own `SSH_CREDENTIALS_DIR` and pins the host key on every connection, so a server missing either value is refused rather than connected to unverified. Private key material never reaches the control plane, which means an API database compromise does not yield fleet access. Key rotation is still a file operation on each runner: automated rotation, secret-manager-backed provisioning, and certificate-based host trust remain future work.
- **Host key changes require operator action.** A legitimately rebuilt host presents a new key, and the runner refuses it until the new fingerprint is recorded. This is deliberate, but there is no assisted re-pinning flow yet.
- **Single runner.** The MVP supports one runner per organisation. Multiple runner federations not yet supported.
- **Runner credentials are runner-bound.** A bootstrap registration credential is valid for one hour and is only good for the registration handshake, which exchanges it for a credential bound to that runner identity. The job endpoints (claim, renew, result, heartbeat) accept only the bound form, so one runner cannot act as another within an organisation. Bound credentials last 30 days and are replaced by re-registering; issuing a replacement or revoking the runner invalidates the previous credential. Secret-manager-backed provisioning and self-service rotation remain future work.
- **Dispatched jobs are signed.** The API authenticates every job over the command, host, port, user, timeout, credential reference, host key fingerprint, and lease, and the runner refuses any job it cannot verify, reporting the rejection instead of executing. `JOB_SIGNING_KEY` must be identical on both sides. Key rotation is a coordinated restart; overlapping key windows are not supported.
- **JetStream is an optional notification bridge.** With `JOB_DISPATCH=jetstream`, durable pull consumers wake runners with metadata-only notifications. The API database lease remains authoritative and runners still claim through `GET /api/v1/jobs/next`, so duplicate delivery cannot create a second lease. A fully independent queue with separate scheduling and horizontally scaled worker control remains future work.
- **No interactive SSH.** Only non-interactive command execution. No TTY, no session recording.
- **Self-contained output storage is the default.** Large stdout/stderr is encrypted in the local artefact directory and referenced from SQLite. S3-compatible storage remains an optional extension for larger deployments.
- **Database dispatch is the default.** Leases, bounded attempt metadata, durable result receipts, and persisted request idempotency keys are present for recovery and safe replay. The JetStream bridge is an optional wake-up path. Exactly-once execution of arbitrary shell side effects after a runner power loss still requires command-level idempotency or a durable runner outbox.
- **Automation is interval-based only.** The API and embedded scheduler can run published low and medium risk runbooks on a fixed interval. High and critical risk schedules are rejected from unattended execution until approval-backed automation is implemented. Senior operators can pause and resume new scheduled work across the organisation, with audit records for both actions.
- **Automation does not yet provide full closed-loop operations.** Event triggers, maintenance windows, execution verification, rollback, notifications, and escalation rules remain on the product improvement backlog.

## RBAC & Policy

- **Fixed roles only.** Owner, admin, senior_engineer, junior_engineer, auditor. No custom roles.
- **No OpenFGA.** Relationship-based authorization uses internal role checks. OpenFGA planned post-MVP.
- **No OPA.** Policy is evaluated by a simple Go-based evaluator. Rego/policy-as-code deferred.
- **Limited risk classification.** Command risk is pattern-matched. No custom risk profiles.

## Runbooks & Approvals

- **41 runbook templates** included across diagnostics (7), maintenance (6), security & performance (7), recovery (4), provisioning (7), AI stack (6), and examples (4).
- **Flat YAML.** Runbooks are single-command with parameter substitution. No branching, no multi-step workflows.
- **No Git sync.** Runbooks live in the database only. Git-backed runbooks planned.
- **Approval expiry is installation-wide.** It defaults to one hour and can be changed with `APPROVAL_EXPIRY_SECONDS`, but per-policy and per-risk expiry windows are not yet supported.
- **No delegation chains.** Approvals are single-level. No multi-level approval workflows.
- **AI assistance is bounded and read-only.** The API, CLI, web console, SDK, and MCP can analyse supplied evidence or redacted execution output through an explicitly configured OpenAI-compatible or local model provider. AI cannot queue infrastructure work. Conversation history, retrieval across an organisation, model administration, streaming responses, and provider failover remain future work.
- **MCP is a local stdio integration.** The MCP server exposes the current API capabilities and is intended to run on a trusted host. It does not yet provide hosted MCP transport, streaming execution output, dynamic resources, or a packaged installer. Writes are disabled by default and remain limited to API-backed actions.

## Infrastructure

- **The extended tier is only partly runtime-enabled.** PostgreSQL metadata now starts through the versioned Goose migrations and a live schema contract check. Optional PostgreSQL RLS can be enabled with `POSTGRES_RLS=true`, which pins authenticated requests to a tenant connection and applies policies to business tables. S3-compatible artifact storage can be selected when its complete configuration validates, and JetStream can be selected as the database-authoritative notification bridge. External scheduling, NATS event publishing, and an independent horizontally scaled queue remain fail-closed or incomplete. The self-contained SQLite, local artefact, database polling, and embedded scheduler tier remains the default.
- **S3 storage has a deliberately narrow support boundary.** The API composes the isolated S3-compatible store with client-side encryption, optional S3 server-side encryption, retries, and checksum verification. PostgreSQL metadata, JetStream dispatch, external scheduling, and NATS events are not enabled by selecting S3.
- **S3 deployments need an S3 retention plan.** The `artifact-migrate` helper copies local encrypted artefacts to S3, preserves IDs, writes a durable object manifest, verifies checksums, and refuses conflicts by default. Extended deployments can use `extended-backup.sh` and `extended-restore.sh` to retain a PostgreSQL dump with the S3 manifest and verify both stores together. S3 versioning, lifecycle policy, encryption-key custody, and off-host recovery remain deployment responsibilities and must be included in the host rehearsal.
- **PostgreSQL is available as an opt-in metadata backend.** The API applies the checked-in migrations, verifies the live catalog, and uses the shared SQL runtime for current handlers. A production rollout still needs a measured migration rehearsal, concurrency testing, and target-host recovery evidence. RLS should be enabled for production PostgreSQL deployments by a migration-owner API bootstrap, then the API should run with a separate non-owner role. Startup refuses to claim RLS readiness when that policy-owner bootstrap has not happened.

- **SQLite is the default deployment database.** PostgreSQL is an optional extension for higher concurrency and horizontal scaling. SQLite remains single-writer and does not provide HA.
- **Backup is local-first.** `make backup` creates a SQLite backup, encrypted artefact copy, and checksum manifest. `backup-verify` and `backup-restore` support local recovery, and the systemd deployment can run a daily retained backup with a journal/webhook failure hook. Extended deployments have a combined PostgreSQL and S3 verification helper. Signed release evidence, object-store retention controls, and measured production RPO/RTO still require deployment-specific rehearsal.
- **Single binary API.** No horizontal scaling. The API is a single process.
- **Monitoring is deployment-owned.** The API exposes `/api/v1/health`, `/api/v1/ready`, and a bounded `/metrics` endpoint with request, queue, dead-letter, active-runner, enabled-schedule, and local artefact-filesystem capacity metrics. The runner can expose loopback-only `/health` and `/metrics` endpoints when `RUNNER_HEALTH_ADDR` is configured. Readiness verifies database connectivity and encrypted artefact-store access. The services emit request IDs and structured logs, and the systemd package includes local health, backup, and backup-freshness timers. Prometheus scrape and alert examples are included, but external collection, alert routing, and incident testing must still be configured per installation.
- **The schema is written twice, once per dialect.** SQLite uses an embedded `CREATE TABLE` schema in `apps/api/migrate.go` while PostgreSQL uses versioned Goose migrations in `migrations/postgres/`. Every new table or column must be added to both. `TestSchemaParityIsComplete` derives both schemas and fails the build on any table or column that exists in only one of them, so drift is caught rather than discovered in production. It does not compare column types: the PostgreSQL schema deliberately keeps `INTEGER` boolean columns (`automation_schedules.enabled`, `automation_controls.paused`) so the handlers can share one SQL string across both dialects. Collapsing to a single generated source of truth remains outstanding.

## CLI & TUI

- **The generated ConnectRPC service stubs are not the active server transport.** The supported CLI and SDK use the documented HTTP API client. The protobuf and ConnectRPC packages remain a contract foundation until the API handlers are wired to that transport.
- **TUI workflow coverage is incomplete.** The TUI provides a guided runbook target, reason, parameter, preflight, submit flow, approval brief detail, approval actions with denial notes, confirmed cancellation of queued executions, and guided schedule creation and disabling. The TUI still does not provide a full task inbox, asynchronous request progress, or live execution recovery experience. See the [Product Improvement Plan](PRODUCT_IMPROVEMENT_PLAN.md).
- **Runbook search is client-side only.** The TUI runbook list supports `/` key filtering on name/title/description/risk. The API supports `?search=` query for server-side matching.
- **No shell completion.** Autocomplete not generated.
- **Windows CRLF warnings.** Cosmetic git warnings, no functional impact.

## Web Console

- **Development identity switching is not production authentication.** With `VPS_DEV_AUTH=true`, the web console proxy sets the `X-VPS-User` header on behalf of the caller (overridable with `VPS_DEV_USER`) and strips any identity headers the browser supplied. It requires both `VPS_DEV_AUTH` and `NEXT_PUBLIC_DEV_AUTH`, and the API additionally refuses header identity outside an explicitly non-production environment. Production deployments should use the OIDC login and session flow.
- **No SSR/SSG.** Client-side only. Data fetched on tab switch, no pre-rendering.
- **No responsive design.** Optimized for desktop.

## Security

- **Audit hash-chain keys are local.** Audit events are now chained per organisation and can be verified through the API. The chain does not replace protected off-host log export or an external key-management and retention policy.
- **Output is redacted at the API boundary as well as by the runner.** Large output is encrypted in the local artefact store.
- **Rate limiting is process-local.** Mutating API requests and authentication mutations have bounded in-process limits. Distributed limits, edge enforcement, and account-specific lockout policies remain future work.
- **CORS is allowlisted but basic.** Set `VPS_WEB_ORIGIN` to the exact console origin. The web proxy checks the origin on state-changing requests, while a broader CSRF token scheme remains future work.

## What Works

Despite these limitations, the MVP delivers the core value proposition:
- Register servers via CLI
- Run controlled commands through the runner
- Create and publish versioned runbooks
- Execute runbooks (juniors can run permitted ones)
- Enforce role-based access (senior vs junior)
- Require reasons for production actions
- Require approvals for high-risk production runbooks
- Full audit trail of all sensitive actions
- CLI, TUI (with `/` runbook search), and web console access
