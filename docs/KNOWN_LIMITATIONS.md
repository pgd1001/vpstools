# VPS Tools — Known Limitations (MVP)

## Authentication & Identity

- **Development auth is header-based.** Set `VPS_DEV_AUTH=true` only for local demos. Production web access supports the configured OIDC session flow. CLI and API deployments still need a production identity and token integration beyond the local development header.
- **No password/token storage.** Production use requires an identity provider.

## Execution & Runner

- **Simulated SSH when Docker unavailable.** Set `SIMULATE=true` for local dev without real SSH target.
- **Single runner.** The MVP supports one runner per organisation. Multiple runner federations not yet supported.
- **No NATS.** Jobs are dispatched via database polling (`GET /api/v1/jobs/next`). NATS JetStream planned post-MVP.
- **No interactive SSH.** Only non-interactive command execution. No TTY, no session recording.
- **Self-contained output storage is the default.** Large stdout/stderr is encrypted in the local artefact directory and referenced from SQLite. S3-compatible storage remains an optional extension for larger deployments.
- **Database dispatch is the default.** Leases and bounded attempt metadata are present for recovery. JetStream remains an optional extension.
- **Automation is interval-based only.** The API and embedded scheduler can run published low and medium risk runbooks on a fixed interval. High and critical risk schedules are rejected from unattended execution until approval-backed automation is implemented.
- **No automation pause, event triggers, verification, rollback, notifications, or escalation rules.** These remain on the product improvement backlog.

## RBAC & Policy

- **Fixed roles only.** Owner, admin, senior_engineer, junior_engineer, auditor. No custom roles.
- **No OpenFGA.** Relationship-based authorization uses internal role checks. OpenFGA planned post-MVP.
- **No OPA.** Policy is evaluated by a simple Go-based evaluator. Rego/policy-as-code deferred.
- **Limited risk classification.** Command risk is pattern-matched. No custom risk profiles.

## Runbooks & Approvals

- **41 runbook templates** included across diagnostics (7), maintenance (6), security & performance (7), recovery (4), provisioning (7), AI stack (6), and examples (4).
- **Flat YAML.** Runbooks are single-command with parameter substitution. No branching, no multi-step workflows.
- **No Git sync.** Runbooks live in the database only. Git-backed runbooks planned.
- **Approval expiry is 1 hour hardcoded.** Configurable expiry deferred.
- **No delegation chains.** Approvals are single-level. No multi-level approval workflows.
- **AI provider boundary only.** The code has a vendor-neutral provider contract and redaction wrapper, but there is no user-facing AI assistant, configured model adapter, evidence retrieval service, or local-model administration workflow yet.

## Infrastructure

- **SQLite is the default deployment database.** PostgreSQL is an optional extension for higher concurrency and horizontal scaling. SQLite remains single-writer and does not provide HA.
- **Backup is local-first.** `make backup` creates a SQLite backup, encrypted artefact copy, and manifest. Automated scheduling and remote backup replication remain future work.
- **Single binary API.** No horizontal scaling. The API is a single process.
- **No health monitoring.** Runner and server health checks are manual (`vps server check`).

## CLI & TUI

- **TUI workflow coverage is incomplete.** The TUI supports approval actions, but it does not yet provide the full guided task workflow, approval detail view, parameter form, or live execution recovery experience. See the [Product Improvement Plan](PRODUCT_IMPROVEMENT_PLAN.md).
- **Runbook search is client-side only.** The TUI runbook list supports `/` key filtering on name/title/description/risk. The API supports `?search=` query for server-side matching.
- **No shell completion.** Autocomplete not generated.
- **Windows CRLF warnings.** Cosmetic git warnings, no functional impact.

## Web Console

- **Development identity switching is not production authentication.** With `VPS_DEV_AUTH=true`, the web console uses the `X-VPS-User` header and local storage selector. Production deployments should use the OIDC login and session flow.
- **No SSR/SSG.** Client-side only. Data fetched on tab switch, no pre-rendering.
- **No responsive design.** Optimized for desktop.

## Security

- **No hash-chain audit.** Audit events are append-only at application level, not cryptographically chained.
- **Output is redacted at the API boundary as well as by the runner.** Large output is encrypted in the local artefact store.
- **No rate limiting.** No brute-force protection on endpoints.
- **CORS is allowlisted but basic.** Set `VPS_WEB_ORIGIN` to the exact console origin. OIDC and CSRF protection are still pending.

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
