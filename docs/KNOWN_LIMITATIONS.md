# VPS Tools: Known Limitations (MVP)

## Authentication & Identity

- **Development auth is header-based.** Set `VPS_DEV_AUTH=true` only for local demos. Production web access supports the configured OIDC session flow, and CLI, SDK, and automation clients can use expiring API bearer tokens. A direct CLI OIDC login flow remains future work.
- **API tokens are operator-managed.** Tokens are stored as hashes, expire, can be revoked, and are shown once. A secret manager integration and token self-service UI remain future work.

## Execution & Runner

- **Simulated SSH when Docker unavailable.** Set `SIMULATE=true` for local dev without real SSH target.
- **Single runner.** The MVP supports one runner per organisation. Multiple runner federations not yet supported.
- **Runner credentials are short-lived and runner-bound.** Registration credentials expire after one hour. Issuing a replacement or revoking its runner invalidates the previous credential. Secret-manager-backed provisioning and self-service credential rotation remain future work.
- **No NATS.** Jobs are dispatched via database polling (`GET /api/v1/jobs/next`). NATS JetStream planned post-MVP.
- **No interactive SSH.** Only non-interactive command execution. No TTY, no session recording.
- **Self-contained output storage is the default.** Large stdout/stderr is encrypted in the local artefact directory and referenced from SQLite. S3-compatible storage remains an optional extension for larger deployments.
- **Database dispatch is the default.** Leases, bounded attempt metadata, durable result receipts, and persisted request idempotency keys are present for recovery and safe replay. JetStream remains an optional extension. Exactly-once execution of arbitrary shell side effects after a runner power loss still requires command-level idempotency or a durable runner outbox.
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
- **AI provider boundary only.** The code has a vendor-neutral provider contract and redaction wrapper, but there is no user-facing AI assistant, configured model adapter, evidence retrieval service, or local-model administration workflow yet.
- **MCP is a local stdio integration.** The MCP server exposes the current API capabilities and is intended to run on a trusted host. It does not yet provide hosted MCP transport, streaming execution output, dynamic resources, or a packaged installer. Writes are disabled by default and remain limited to API-backed actions.

## Infrastructure

- **The extended tier is not runtime-enabled yet.** PostgreSQL, S3-compatible storage, JetStream, external scheduling, and NATS event settings are validated as configuration targets, but the current API intentionally refuses to start with them selected. The live implementation remains the self-contained SQLite, local artefact, database polling, and embedded scheduler tier. Do not treat a configuration file containing external URLs as evidence that those adapters are supported.

- **SQLite is the default deployment database.** PostgreSQL is an optional extension for higher concurrency and horizontal scaling. SQLite remains single-writer and does not provide HA.
- **Backup is local-first.** `make backup` creates a SQLite backup, encrypted artefact copy, and checksum manifest. `backup-verify` and `backup-restore` support local recovery, and the systemd deployment can run a daily retained backup with a journal/webhook failure hook. A separately mounted replication destination is supported, but signed manifests, object-store replication, and measured production RPO/RTO remain future work.
- **Single binary API.** No horizontal scaling. The API is a single process.
- **Monitoring is deployment-owned.** The API exposes `/api/v1/health`, `/api/v1/ready`, and a bounded `/metrics` endpoint with request, queue, dead-letter, active-runner, enabled-schedule, and local artefact-filesystem capacity metrics. The runner can expose loopback-only `/health` and `/metrics` endpoints when `RUNNER_HEALTH_ADDR` is configured. Readiness verifies database connectivity and encrypted artefact-store access. The services emit request IDs and structured logs, and the systemd package includes local health, backup, and backup-freshness timers. Prometheus scrape and alert examples are included, but external collection, alert routing, and incident testing must still be configured per installation.

## CLI & TUI

- **The generated ConnectRPC service stubs are not the active server transport.** The supported CLI and SDK use the documented HTTP API client. The protobuf and ConnectRPC packages remain a contract foundation until the API handlers are wired to that transport.
- **TUI workflow coverage is incomplete.** The TUI provides a guided runbook target, reason, parameter, preflight, submit flow, approval brief detail, approval actions with denial notes, confirmed cancellation of queued executions, and guided schedule creation and disabling. The TUI still does not provide a full task inbox, asynchronous request progress, or live execution recovery experience. See the [Product Improvement Plan](PRODUCT_IMPROVEMENT_PLAN.md).
- **Runbook search is client-side only.** The TUI runbook list supports `/` key filtering on name/title/description/risk. The API supports `?search=` query for server-side matching.
- **No shell completion.** Autocomplete not generated.
- **Windows CRLF warnings.** Cosmetic git warnings, no functional impact.

## Web Console

- **Development identity switching is not production authentication.** With `VPS_DEV_AUTH=true`, the web console uses the `X-VPS-User` header and local storage selector. Production deployments should use the OIDC login and session flow.
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
