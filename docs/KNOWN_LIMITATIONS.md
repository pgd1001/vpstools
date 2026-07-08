# VPS Tools — Known Limitations (MVP)

## Authentication & Identity

- **Dev auth only.** The MVP uses `X-VPS-User` header for identity. No OIDC, no SSO, no MFA, no session tokens.
  OIDC integration is planned post-MVP.
- **No password/token storage.** Production use requires an identity provider.

## Execution & Runner

- **Simulated SSH when Docker unavailable.** Set `SIMULATE=true` for local dev without real SSH target.
- **Single runner.** The MVP supports one runner per organisation. Multiple runner federations not yet supported.
- **No NATS.** Jobs are dispatched via database polling (`GET /api/v1/jobs/next`). NATS JetStream planned post-MVP.
- **No interactive SSH.** Only non-interactive command execution. No TTY, no session recording.
- **No output retention.** stdout/stderr stored inline in database. Object storage (MinIO/S3) planned post-MVP.
- **No retry logic.** Failed executions are not automatically retried.

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

## Infrastructure

- **SQLite for local dev.** PostgreSQL required for production. SQLite is single-writer, no HA.
- **No backup/restore.** Manual database file copy is the only backup mechanism.
- **Single binary API.** No horizontal scaling. The API is a single process.
- **No health monitoring.** Runner and server health checks are manual (`vps server check`).

## CLI & TUI

- **TUI is read-only.** The TUI displays data but does not support in-screen operations (approve/deny works in web console).
- **Runbook search is client-side only.** The TUI runbook list supports `/` key filtering on name/title/description/risk. The API supports `?search=` query for server-side matching.
- **No shell completion.** Autocomplete not generated.
- **Windows CRLF warnings.** Cosmetic git warnings, no functional impact.

## Web Console

- **No authentication.** The web console uses the same `X-VPS-User` header / localStorage mechanism.
- **No SSR/SSG.** Client-side only. Data fetched on tab switch, no pre-rendering.
- **No responsive design.** Optimized for desktop.

## Security

- **No hash-chain audit.** Audit events are append-only at application level, not cryptographically chained.
- **No output redaction server-side.** Redaction happens in the runner before submitting to API. API does not redact.
- **No rate limiting.** No brute-force protection on endpoints.
- **No CORS configuration.** CORS headers not set on the API.

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
