# VPS Tools MVP Build Plan

**Working title:** VPS Tools  
**Document status:** Draft v1  
**Date:** 18 May 2026  
**Related documents:** VPS Tools PRD; VPS Tools Technical Specification  
**Primary audience:** Founder, engineering, product, DevOps, security  

---

## 1. Purpose

This document turns the VPS Tools PRD and technical specification into a practical MVP execution plan.

The goal is not to describe the whole future product. The goal is to define the shortest credible path to a working, testable, secure MVP that proves the core product promise:

> Safely delegate and audit VPS operations from the CLI.

The MVP must prove that VPS Tools can:

1. Register and manage VPS inventory.
2. Execute controlled operations through a runner.
3. Enforce role and policy checks.
4. Capture complete audit evidence.
5. Support a basic delegated runbook workflow.
6. Run as both a local self-hosted stack and a SaaS-ready architecture.

---

## 2. MVP Definition

The MVP is complete when a small DevOps team can use VPS Tools to manage a small fleet of Linux VPS servers through the CLI with basic governance and auditability.

### 2.1 MVP Product Statement

VPS Tools MVP is a CLI/TUI-first control plane that allows a senior engineer to register VPS servers, run approved commands, create basic runbooks, delegate safe tasks to junior engineers, require approval for risky actions, and review an audit trail of all sensitive activity.

### 2.2 MVP Technical Statement

The MVP is a Go-based system composed of:

- CLI/TUI app.
- Control plane API.
- Execution runner.
- PostgreSQL database.
- Object storage for execution output.
- Basic authorisation and policy layer.
- Docker Compose self-hosted deployment.
- Minimal web console for administration, approvals, and audit review.

### 2.3 First Working Vertical Slice

The first working vertical slice is:

> Register a server, run `uptime` through the runner, enforce a basic permission check, capture stdout/stderr/exit code, and write an audit event.

This is the first major milestone because it proves the core architecture:

- CLI can talk to API.
- API can authenticate and authorise the user.
- API can create an execution.
- Runner can receive the job.
- Runner can SSH to a server.
- Runner can execute a command.
- Output can be captured.
- Audit event can be recorded.

Everything else builds from this.

---

## 3. MVP Scope

## 3.1 In Scope

### Core CLI

- `vps login`
- `vps logout`
- `vps whoami`
- `vps server add`
- `vps server list`
- `vps server inspect`
- `vps server check`
- `vps exec`
- `vps run`
- `vps runbook list`
- `vps approvals list`
- `vps approvals approve`
- `vps approvals deny`
- `vps audit search`
- `vps audit show`
- `vps tui` basic shell

### Basic TUI

- Server browser.
- Runbook launcher.
- Execution monitor.
- Approval queue.
- Audit event viewer.
- Setup wizard.

### Control Plane API

- Authentication/session support.
- Organisation model.
- User and membership model.
- Server inventory.
- Runner registration.
- Execution creation and tracking.
- Basic policy checks.
- Approval lifecycle.
- Audit event creation and search.
- Runbook CRUD and versioning.

### Runner

- Runner registration.
- Runner heartbeat.
- Job polling or streaming.
- SSH command execution.
- Timeout handling.
- stdout/stderr/exit code capture.
- Result submission.
- Output upload to object storage.

### Inventory

- Add server manually.
- Store hostname/IP, SSH profile, OS metadata, environment, tags, and status.
- List and inspect servers.
- Basic health check.
- Tag-based grouping.

### Execution

- Single-server command execution.
- Group command execution.
- Basic concurrency limits.
- Execution status tracking.
- Per-target result.
- Output capture and retrieval.

### Runbooks

- YAML runbook schema.
- Basic parameter validation.
- Target constraints.
- Execution settings.
- Versioned publication.
- Runbook execution from CLI.
- Runbook execution audit trail.

### RBAC and Policy

- Roles: Owner, Admin, Senior Engineer, Junior Engineer, Auditor.
- Basic role checks.
- Basic target/environment checks.
- Junior users limited to delegated runbooks.
- Senior users allowed to run authorised raw commands.
- Production actions require reason.
- Selected risky actions require approval.

### Approvals

- Approval request creation.
- Approval list.
- Approve/deny decision.
- Expiry.
- Link approval to execution.
- Audit approval decisions.

### Audit

- Append-only application-level audit event model.
- Audit event on every sensitive action.
- Search by actor, action, target, environment, result, and date.
- Audit event detail view.
- Basic export may be included if time allows.

### Self-Hosted Deployment

- Docker Compose stack.
- PostgreSQL.
- MinIO or local S3-compatible object storage.
- API.
- Runner.
- Web console.
- Optional OpenFGA if included in MVP cut.

### SaaS Readiness

- Organisation isolation.
- Tenant-aware data model.
- Runner designed to connect outbound.
- Clear separation between control plane and runner.
- Plan/licence placeholders, but not full billing.

---

## 3.2 Out of Scope for MVP

The following should not be built in the MVP unless required by a committed beta customer:

- Full terminal session recording.
- SSH CA as the only access model.
- Full OPA/Rego policy-as-code.
- Kubernetes/Helm production deployment.
- Provider auto-discovery.
- Full SIEM integrations.
- Advanced compliance packs.
- MSP multi-client reporting.
- AI-assisted runbook generation.
- Full billing system.
- Full marketplace or plugin ecosystem.
- Advanced drift detection.
- Patch orchestration waves.
- Reboot orchestration.
- Backup platform functionality.
- Mobile app.

---

## 4. Build Principles

### 4.1 Build Vertical Slices

Do not build all of the database, then all of the API, then all of the CLI. Build thin vertical slices that prove real user workflows end to end.

Preferred order:

1. Minimal local stack.
2. Register server.
3. Execute command.
4. Capture audit.
5. Add role checks.
6. Add runbook.
7. Add approval.
8. Add TUI polish.

### 4.2 Keep the First Version Boring

The first version should be technically sound but not over-engineered.

Use:

- Go.
- PostgreSQL.
- Docker Compose.
- Simple runner job loop.
- Simple structured policy evaluator.
- Simple object storage.
- Clear audit model.

Avoid premature complexity:

- Overly abstract plugin systems.
- Highly dynamic policy DSLs.
- Multi-region SaaS architecture.
- Full enterprise deployment tooling.
- Complex event sourcing.
- Full-blown workflow engine.

### 4.3 Security Cannot Be Bolted On Later

The MVP can be limited, but it must not be casual with privileged access.

Non-negotiables:

- Deny by default.
- No shared user accounts inside VPS Tools.
- Every privileged action audited.
- Runner cannot authorise itself.
- CLI cannot bypass API policy.
- No secrets in logs.
- Clear separation between junior and senior permissions.

### 4.4 Defer Features, Not Foundations

It is acceptable to defer terminal recording, SSO polish, SIEM integrations, or provider discovery.

It is not acceptable to defer:

- Organisation scoping.
- Audit event design.
- Runner trust boundary.
- Role model.
- Execution lifecycle.
- Secure token handling.

---

## 5. Recommended Timeline

This plan assumes one strong full-stack/founding engineer, with occasional product/security review. With a small team of two to three engineers, phases can overlap.

| Phase | Name | Target duration | Outcome |
|---|---:|---:|---|
| 0 | Architecture Spike | 1 week | Prove CLI/API/runner/SSH/audit path |
| 1 | Foundations | 1–2 weeks | Monorepo, local stack, auth stub, database, CI |
| 2 | Inventory and Runner | 1–2 weeks | Add servers, register runner, health check |
| 3 | Execution Engine | 2 weeks | Reliable command execution and result capture |
| 4 | RBAC and Policy | 2 weeks | Role enforcement and basic policy decisions |
| 5 | Runbooks and Approvals | 2 weeks | Delegated runbooks and approval flow |
| 6 | TUI and Web Console | 2 weeks | Usable operator UX and admin review UX |
| 7 | Hardening and Beta | 2–3 weeks | Security review, docs, packaging, beta readiness |

Indicative total: **13–16 weeks** for a credible private beta.

This is not a promise. It is a planning baseline. The riskiest areas are runner security, SSH edge cases, authorisation model quality, and UX polish.

---

## 6. Phase 0: Architecture Spike

### 6.1 Goal

Prove the core technical path before investing in the full product skeleton.

### 6.2 Target Outcome

A developer can run a local stack and execute:

```bash
vps exec server:demo -- uptime
```

The command should:

1. Call the API.
2. Create an execution record.
3. Pass a basic authorisation check.
4. Send a job to the runner.
5. SSH into a test VPS/container.
6. Run `uptime`.
7. Capture stdout, stderr, and exit code.
8. Store the result.
9. Write an audit event.
10. Return the result to the CLI.

### 6.3 Deliverables

- Minimal Go monorepo.
- Basic Cobra CLI.
- Minimal Bubble Tea TUI proof of concept.
- Minimal ConnectRPC API proof of concept.
- PostgreSQL running locally.
- Basic migrations.
- Local fake VPS container with SSH enabled.
- Minimal runner.
- Basic execution table.
- Basic audit event table.
- Docker Compose dev stack.

### 6.4 Acceptance Criteria

- `make dev-up` starts the local dependencies.
- `vps whoami` returns a seeded local user.
- `vps server list` returns a seeded demo server.
- `vps exec server:demo -- uptime` returns real output.
- Execution result is stored in PostgreSQL.
- Audit event is written.
- Runner logs show job claim and completion.

### 6.5 Phase 0 Backlog

| ID | Task | Notes |
|---|---|---|
| P0-001 | Create monorepo skeleton | `apps/cli`, `apps/api`, `apps/runner`, `packages/proto`, `migrations` |
| P0-002 | Create Docker Compose dev stack | PostgreSQL plus fake SSH target first |
| P0-003 | Add Cobra CLI skeleton | `vps`, `vps whoami`, `vps server list`, `vps exec` |
| P0-004 | Add basic Bubble Tea screen | Enough to validate Charm stack |
| P0-005 | Add ConnectRPC API skeleton | Health and execution endpoint |
| P0-006 | Add initial PostgreSQL migrations | users, organisations, servers, executions, audit_events |
| P0-007 | Add fake auth context | Seeded user and org, no production auth yet |
| P0-008 | Add runner skeleton | Poll or stream jobs from API |
| P0-009 | Add SSH execution helper | Execute command and capture output |
| P0-010 | Add audit event write | Execution requested and completed |

---

## 7. Phase 1: Foundations

### 7.1 Goal

Build the real project foundation so future work is not throwaway.

### 7.2 Deliverables

- Production-quality monorepo structure.
- CI pipeline.
- Code formatting and linting.
- Database migration tooling.
- sqlc query generation.
- Protobuf generation.
- API configuration system.
- CLI configuration system.
- Local development seed data.
- Basic documentation.

### 7.3 Acceptance Criteria

- A new developer can clone the repo and run the local stack from documented commands.
- CI runs tests, linting, and basic security checks.
- Migrations can be applied and rolled back locally.
- Protobuf and sqlc generation are repeatable.
- CLI, API, and runner share generated API contracts.

### 7.4 Phase 1 Backlog

| ID | Task | Notes |
|---|---|---|
| P1-001 | Finalise monorepo layout | Match technical spec |
| P1-002 | Add `make` or `task` commands | `dev-up`, `dev-down`, `migrate`, `test`, `lint`, `generate` |
| P1-003 | Add GitHub Actions CI | Tests, linting, govulncheck, container scan later |
| P1-004 | Add Go module structure | Shared packages for audit, authz, runbooks, sshx |
| P1-005 | Add protobuf generation | ConnectRPC Go and TS clients |
| P1-006 | Add sqlc | Type-safe DB access |
| P1-007 | Add migration tool | Goose or Atlas |
| P1-008 | Add structured logging | API and runner |
| P1-009 | Add configuration loading | Environment variables and config files |
| P1-010 | Add developer documentation | Local setup, architecture overview |

---

## 8. Phase 2: Inventory and Runner

### 8.1 Goal

Allow users to register servers, inspect them, and connect them to a scoped runner.

### 8.2 Deliverables

- Server add/list/inspect/check.
- Server tags.
- Basic dynamic filtering.
- Runner registration.
- Runner heartbeat.
- SSH connection validation.
- Basic OS metadata collection.

### 8.3 Acceptance Criteria

- A senior user can add a server manually.
- The runner can validate SSH connectivity.
- `vps server check server:web-01` returns status and metadata.
- Servers can be filtered by environment and tags.
- Runner status is visible from CLI and web/API.

### 8.4 Phase 2 Backlog

| ID | Task | Notes |
|---|---|---|
| P2-001 | Implement server schema | name, hostname, IP, environment, tags, SSH profile reference |
| P2-002 | Implement `vps server add` | Non-interactive first |
| P2-003 | Implement TUI server add form | Charm Huh |
| P2-004 | Implement `vps server list` | Table and JSON output |
| P2-005 | Implement `vps server inspect` | Metadata and connection status |
| P2-006 | Implement tags | Key/value tags |
| P2-007 | Implement basic server filtering | environment, role, provider, tag |
| P2-008 | Implement runner registration token | Expiring registration token |
| P2-009 | Implement runner heartbeat | last_seen_at and status |
| P2-010 | Implement server health check | uptime, OS, kernel, disk summary |

---

## 9. Phase 3: Execution Engine

### 9.1 Goal

Make command execution reliable, auditable, and understandable.

### 9.2 Deliverables

- Execution creation.
- Target resolution.
- Job queue or job polling.
- Runner job claim.
- SSH command execution.
- stdout/stderr/exit code capture.
- Object storage output upload.
- Execution streaming or polling.
- Per-target result.
- Timeout and cancellation.

### 9.3 Acceptance Criteria

- A senior engineer can run a command against one server.
- A senior engineer can run a command against a small group.
- CLI shows progress and final status.
- Each target has a separate result.
- Output is stored and retrievable.
- Failed targets are clearly reported.
- Every execution creates audit events.

### 9.4 Phase 3 Backlog

| ID | Task | Notes |
|---|---|---|
| P3-001 | Implement execution schema | executions, execution_targets, execution_events |
| P3-002 | Implement execution API | create, get, stream/list events, cancel |
| P3-003 | Implement target resolver | server and tag/group targeting |
| P3-004 | Implement job dispatch | Simple DB polling acceptable for early MVP; NATS can follow |
| P3-005 | Implement runner job claim | Locking and expiry |
| P3-006 | Implement SSH command execution | Timeout, stdout, stderr, exit code |
| P3-007 | Implement object storage output | MinIO locally |
| P3-008 | Implement `vps exec` | Direct command mode |
| P3-009 | Implement execution monitor | CLI progress view |
| P3-010 | Implement cancellation | Best-effort cancellation |
| P3-011 | Implement per-target concurrency | Safe defaults |
| P3-012 | Implement execution audit events | requested, queued, started, completed, failed |

---

## 10. Phase 4: RBAC and Policy

### 10.1 Goal

Make the product safe enough for delegated use.

### 10.2 Deliverables

- Role model.
- Membership model.
- Basic OpenFGA or internal relationship checks.
- Structured policy evaluator.
- Deny-by-default execution rules.
- Junior versus senior behaviour.
- Production reason requirement.
- Policy denial messages.

### 10.3 Acceptance Criteria

- Junior users cannot run arbitrary commands.
- Senior users can run authorised raw commands.
- Production actions require a reason.
- A denied action returns a clear explanation.
- Audit events record denied attempts.
- Policy checks happen server-side, not only in the CLI.

### 10.4 Phase 4 Backlog

| ID | Task | Notes |
|---|---|---|
| P4-001 | Implement users and memberships | Roles and org scoping |
| P4-002 | Implement basic auth/session model | Dev auth first, OIDC-ready structure |
| P4-003 | Implement role checks | Owner, Admin, Senior, Junior, Auditor |
| P4-004 | Decide OpenFGA MVP depth | Full OpenFGA or internal first-pass with compatible model |
| P4-005 | Implement policy documents | Structured YAML/JSON policies |
| P4-006 | Implement policy evaluator | Role, action, environment, target, approval, reason |
| P4-007 | Implement production reason requirement | `--reason` mandatory for production |
| P4-008 | Implement command risk classification | Basic risk enum first |
| P4-009 | Implement denial audit events | Record actor, attempted action, reason denied |
| P4-010 | Add CLI denial UX | Clear next step: request approval or use permitted runbook |

---

## 11. Phase 5: Runbooks and Approvals

### 11.1 Goal

Prove the delegated operations model.

This is where VPS Tools starts becoming more than a governed SSH wrapper.

### 11.2 Deliverables

- YAML runbook schema.
- Runbook validation.
- Runbook versions.
- Runbook list/inspect/run.
- Basic parameter forms.
- Target constraints.
- Approval request lifecycle.
- Approval decision from CLI and web console.
- Expiring approval grants.

### 11.3 Acceptance Criteria

- Senior engineer can create and publish a runbook.
- Junior engineer can list permitted runbooks.
- Junior engineer can run an allowed staging runbook.
- Junior engineer is blocked from production runbook execution when approval is required.
- Junior engineer can request approval.
- Senior engineer can approve or deny.
- Approved execution runs and is linked to the approval.
- All runbook and approval activity is audited.

### 11.4 Phase 5 Backlog

| ID | Task | Notes |
|---|---|---|
| P5-001 | Define runbook schema | YAML, versioned API kind |
| P5-002 | Implement runbook validator | Parameters, targets, execution, output |
| P5-003 | Implement runbook persistence | runbooks and runbook_versions |
| P5-004 | Implement `vps runbook list` | Only permitted runbooks for current user |
| P5-005 | Implement `vps runbook inspect` | Show docs, params, risk, targets |
| P5-006 | Implement `vps run` | Runbook execution |
| P5-007 | Implement parameter rendering | Direct flags first, TUI forms second |
| P5-008 | Implement target constraints | Tags, environment, server groups |
| P5-009 | Implement approval schema | request, decision, expiry |
| P5-010 | Implement approval API | list, approve, deny |
| P5-011 | Implement approval CLI | `vps approvals list/approve/deny` |
| P5-012 | Implement approval web view | Minimal web console queue |
| P5-013 | Link approval to execution | Required for audit trail |
| P5-014 | Audit runbook lifecycle | created, published, disabled, executed |

---

## 12. Phase 6: TUI and Web Console

### 12.1 Goal

Make the MVP usable by real operators rather than just technically functional.

### 12.2 Deliverables

- Basic TUI home screen.
- Server browser.
- Runbook launcher.
- Execution monitor.
- Approval queue.
- Audit browser.
- Minimal web console for users, servers, approvals, and audit.

### 12.3 Acceptance Criteria

- A user can browse servers interactively.
- A user can select a runbook and target interactively.
- A user can monitor an execution interactively.
- An approver can approve/deny from CLI or web.
- An auditor can find an event from CLI or web.
- The CLI remains scriptable and does not require TUI interaction.

### 12.4 Phase 6 Backlog

| ID | Task | Notes |
|---|---|---|
| P6-001 | Implement TUI shell | Navigation, layout, command palette |
| P6-002 | Implement server browser | Search, filters, details |
| P6-003 | Implement runbook launcher | Select runbook, params, targets |
| P6-004 | Implement execution monitor | Progress and per-target result |
| P6-005 | Implement approval queue | Approve/deny with notes |
| P6-006 | Implement audit browser | Search and detail view |
| P6-007 | Implement setup wizard | First login, first server, first check |
| P6-008 | Implement web console shell | Next.js layout and auth context |
| P6-009 | Implement web server inventory | List and detail |
| P6-010 | Implement web approvals | Queue and decision |
| P6-011 | Implement web audit search | Search and detail |
| P6-012 | Implement basic user management | Invite/role edit if time allows |

---

## 13. Phase 7: Hardening and Private Beta

### 13.1 Goal

Prepare the MVP for use by a small number of trusted beta users.

### 13.2 Deliverables

- Security review.
- Token hardening.
- Secret redaction.
- Output access controls.
- Runner scope hardening.
- Audit consistency checks.
- Basic documentation.
- Docker Compose install guide.
- CLI release artefacts.
- Known limitations document.
- Beta onboarding checklist.

### 13.3 Acceptance Criteria

- No known critical security gaps in the intended beta use case.
- Local self-hosted install works from documented steps.
- CLI binaries can be built and distributed.
- Audit events are complete for MVP workflows.
- A beta user can complete the onboarding path without engineering assistance.
- Known limitations are explicit.

### 13.4 Phase 7 Backlog

| ID | Task | Notes |
|---|---|---|
| P7-001 | Perform threat model review | Use security doc as checklist |
| P7-002 | Harden token storage | OS keyring and expiry |
| P7-003 | Add output redaction | Common token/password patterns |
| P7-004 | Add runner scope checks | Prevent cross-org/cross-scope job claim |
| P7-005 | Add audit completeness tests | Sensitive actions must emit events |
| P7-006 | Add tenant isolation tests | Cross-org access attempts fail |
| P7-007 | Add Docker Compose install guide | Self-hosted open-source base |
| P7-008 | Add first-run guide | Add server, run command, create runbook |
| P7-009 | Add GoReleaser config | CLI binaries |
| P7-010 | Add container build pipeline | API, runner, web |
| P7-011 | Add known limitations doc | Be explicit and honest |
| P7-012 | Prepare private beta checklist | Target users, feedback form, support route |

---

## 14. Initial Data Model Cut

This is the minimum database scope for the MVP. It is intentionally smaller than the long-term model.

### 14.1 Required MVP Tables

```text
organisations
users
memberships
servers
server_tags
runners
runner_scopes
runbooks
runbook_versions
policies
approval_requests
executions
execution_targets
execution_events
audit_events
api_tokens
```

### 14.2 Defer Until Later

```text
teams
team_memberships
billing_accounts
licences
notifications
server_group_memberships
advanced_policy_versions
session_recordings
compliance_reports
```

Teams can be deferred if role and organisation-level scoping are enough for the MVP. Server groups can initially be implemented through tag-based targeting before adding explicit static groups.

---

## 15. Initial API Contract Cut

### 15.1 Required MVP Services

```text
AuthService
OrganisationService
ServerService
RunnerService
ExecutionService
RunbookService
ApprovalService
AuditService
```

### 15.2 Defer Until Later

```text
NotificationService
LicenceService
BillingService
ProviderImportService
ComplianceReportService
SessionRecordingService
```

### 15.3 Minimum API Methods

#### AuthService

```text
WhoAmI
LoginDev
Logout
RefreshToken
```

#### OrganisationService

```text
ListOrganisations
GetOrganisation
SwitchOrganisationContext
```

#### ServerService

```text
AddServer
ListServers
GetServer
UpdateServer
CheckServer
DeleteServerOrArchiveServer
```

#### RunnerService

```text
CreateRunnerRegistrationToken
RegisterRunner
Heartbeat
ListRunners
GetRunner
PollJobs or StreamJobs
SubmitExecutionResult
```

#### ExecutionService

```text
CreateExecution
GetExecution
ListExecutions
StreamExecutionEvents
CancelExecution
GetExecutionOutput
```

#### RunbookService

```text
CreateRunbook
ValidateRunbook
PublishRunbookVersion
ListRunbooks
GetRunbook
DisableRunbook
ExecuteRunbook
```

#### ApprovalService

```text
CreateApprovalRequest
ListApprovalRequests
GetApprovalRequest
ApproveApprovalRequest
DenyApprovalRequest
ExpireApprovalRequests
```

#### AuditService

```text
SearchAuditEvents
GetAuditEvent
ExportAuditEvents
```

---

## 16. Initial CLI Cut

### 16.1 Phase 0 CLI

```bash
vps whoami
vps server list
vps exec server:demo -- uptime
```

### 16.2 MVP CLI

```bash
vps login
vps logout
vps whoami

vps server add
vps server list
vps server inspect <server>
vps server check <server>

vps exec <server|selector> -- <command>

vps runbook list
vps runbook inspect <runbook>
vps run <runbook> --target <server|selector> [params]

vps approvals list
vps approvals approve <approval-id>
vps approvals deny <approval-id>

vps audit search
vps audit show <event-id>

vps runner register
vps runner list
vps runner status

vps tui
```

### 16.3 CLI Output Requirements

Every list/detail command must support:

```bash
--output table
--output json
```

Commands that can affect servers must support:

```bash
--reason "..."
--yes
--dry-run
```

`--yes` should only bypass local confirmation prompts. It must not bypass server-side policy or approval requirements.

---

## 17. Initial TUI Cut

The TUI should not block the core CLI. Build it incrementally.

### 17.1 TUI v0

- Home screen.
- Current user/org.
- List servers.
- List recent executions.
- Quit/help controls.

### 17.2 TUI v1

- Server browser.
- Runbook launcher.
- Approval queue.
- Execution monitor.

### 17.3 TUI v2

- Audit search.
- Setup wizard.
- Runbook parameter forms.
- Better keyboard shortcuts.
- Improved error recovery.

---

## 18. Initial Web Console Cut

The web console should be useful but secondary.

### 18.1 Web v0

- Login/dev auth.
- Organisation context.
- Server list.
- Execution list.
- Audit event list.

### 18.2 Web v1

- Approval queue.
- User/member list.
- Runbook list/detail.
- Audit search/detail.

### 18.3 Web v2

- User invites.
- Role assignment.
- Server detail/edit.
- Runbook create/edit.
- Settings.

---

## 19. Security Gates

The MVP cannot move to private beta unless these gates pass.

### 19.1 Gate A: Authorisation

- Junior cannot run arbitrary commands.
- Junior cannot target production unless policy allows it.
- Senior cannot bypass approval when approval is required.
- Auditor cannot execute commands.
- Removed user loses access.

### 19.2 Gate B: Runner Trust Boundary

- Runner cannot claim jobs from another organisation.
- Runner cannot execute unsigned or expired jobs.
- Runner cannot alter policy decision.
- Runner identity can be revoked.

### 19.3 Gate C: Audit Completeness

- Login/logout audited where implemented.
- Server add/update/check audited.
- Execution requested/started/completed/failed audited.
- Approval requested/approved/denied/expired audited.
- Runbook created/published/executed audited.
- Denied attempts audited.

### 19.4 Gate D: Secret Safety

- CLI token not written to plain config by default.
- SSH private keys not printed in logs.
- Command output redaction covers common secret patterns.
- Debug logs do not expose credentials.

### 19.5 Gate E: Tenant Isolation

- Organisation ID is required on scoped resources.
- Cross-organisation access tests fail.
- List endpoints cannot leak resources across organisations.
- Audit search is organisation-scoped.

---

## 20. Testing Plan

### 20.1 Unit Tests

Required coverage areas:

- Runbook validation.
- Policy evaluator.
- Target resolver.
- Audit event builder.
- CLI output formatting.
- SSH command result parsing.

### 20.2 Integration Tests

Required coverage areas:

- API with PostgreSQL.
- API with object storage.
- Runner job claim and result submission.
- Execution lifecycle.
- Approval lifecycle.
- Audit search.

### 20.3 End-to-End Tests

Required MVP journeys:

1. Senior adds server and runs `uptime`.
2. Junior tries arbitrary command and is denied.
3. Senior creates runbook.
4. Junior runs staging runbook.
5. Junior requests production approval.
6. Senior approves.
7. Approved production execution runs.
8. Auditor finds the full trail.

### 20.4 Security Tests

Required tests:

- Cross-org access attempt.
- Expired token use.
- Revoked runner attempt.
- Runbook command injection attempt.
- Approval bypass attempt.
- Output access by unauthorised user.

---

## 21. Definition of Done

A task is not done until:

- Code is committed.
- Tests are added or explicitly not applicable.
- CLI help/output is updated if needed.
- API contract is regenerated if changed.
- Database migrations are included if schema changed.
- Audit events are added for sensitive actions.
- Errors are clear and actionable.
- Documentation is updated for user-facing behaviour.
- Security impact has been considered.

---

## 22. Private Beta Readiness Checklist

The MVP is ready for private beta when:

- A fresh self-hosted Docker Compose install works from documentation.
- CLI binary can be installed on macOS, Linux, and Windows/WSL.
- A beta user can add a server without developer assistance.
- A beta user can run a command and see the result.
- A senior user can create and run a basic runbook.
- A junior user can run a delegated runbook.
- A production-like action can require approval.
- Audit search shows the full operational timeline.
- Known limitations are documented.
- Security gates pass.
- Backup/restore guidance exists for self-hosted data.
- There is a clear feedback and support route.

---

## 23. First Beta Scenario

Use this as the standard demo and validation path.

### Scenario: Controlled Nginx Restart

Actors:

- Admin/Senior Engineer.
- Junior Engineer.
- Auditor.

Servers:

- `web-01` tagged `env=staging`, `role=web`.
- `web-02` tagged `env=production`, `role=web`.

Flow:

1. Admin creates organisation.
2. Admin adds `web-01` and `web-02`.
3. Admin registers local runner.
4. Senior runs health check on both servers.
5. Senior creates `restart-nginx` runbook.
6. Junior runs `restart-nginx` against staging.
7. Junior attempts to run `restart-nginx` against production.
8. System requires approval.
9. Senior approves with note.
10. System executes production runbook.
11. Auditor searches audit trail and sees the full chain.

This scenario should become an automated end-to-end test and a product demo.

---

## 24. Key Risks During MVP Build

### Risk 1: Runner security becomes too weak

Mitigation:

- Keep runner scoped.
- Sign jobs.
- Require server-side policy decision.
- Audit runner actions.

### Risk 2: TUI polish consumes too much time

Mitigation:

- Build direct CLI first.
- Add TUI only around proven workflows.
- Do not block core functionality on TUI perfection.

### Risk 3: OpenFGA/policy complexity slows the MVP

Mitigation:

- Keep policy model simple.
- Use OpenFGA for relationships if practical.
- Use internal structured policy evaluator first.
- Defer complex policy-as-code.

### Risk 4: SSH edge cases explode scope

Mitigation:

- Support a narrow initial OS set.
- Document required SSH setup.
- Use fake VPS containers for tests.
- Avoid solving every SSH environment in MVP.

### Risk 5: Web console expands too much

Mitigation:

- Web is for admin, approval, and audit.
- CLI remains the primary operations surface.
- Defer dashboards and reports.

### Risk 6: Open-source and commercial boundaries distract from MVP

Mitigation:

- Build open-source base first.
- Add licence/edition hooks without overbuilding billing.
- Defer advanced commercial features until product value is proven.

---

## 25. Immediate Next Actions

### 25.1 Engineering

1. Create repository.
2. Add monorepo structure.
3. Add Docker Compose with PostgreSQL and fake SSH target.
4. Add Go CLI skeleton.
5. Add Go API skeleton.
6. Add Go runner skeleton.
7. Add first database migration.
8. Implement the `uptime` vertical slice.

### 25.2 Product

1. Finalise first beta user persona.
2. Decide whether first beta is aimed at internal DevOps teams, MSPs, or web agencies.
3. Finalise the first demo scenario.
4. Define known limitations for MVP.
5. Draft initial landing page positioning.

### 25.3 Security

1. Create lightweight threat model.
2. Define runner trust boundary.
3. Define audit event minimum fields.
4. Decide initial SSH credential strategy.
5. Decide whether OpenFGA is required in Phase 0 or Phase 4.

---

## 26. Recommended First Sprint

### Sprint Goal

Prove the first working vertical slice.

### Sprint Duration

1 week.

### Sprint Backlog

1. Create repo and monorepo structure.
2. Add Docker Compose with PostgreSQL and fake SSH target.
3. Add CLI skeleton with `vps whoami`, `vps server list`, and `vps exec`.
4. Add API skeleton with seeded dev user/org/server.
5. Add runner skeleton.
6. Add SSH helper to execute `uptime`.
7. Add executions and audit_events tables.
8. Persist execution result.
9. Persist audit event.
10. Document how to run the spike locally.

### Sprint Demo

Run:

```bash
make dev-up
make migrate
make seed
vps whoami
vps server list
vps exec server:demo -- uptime
```

Expected output:

- Current user and organisation shown.
- Demo server listed.
- `uptime` output returned.
- Execution stored.
- Audit event stored.

---

## 27. MVP Success Criteria

The MVP succeeds if it proves:

1. The CLI can make VPS operations faster.
2. The runner can execute work safely enough for controlled beta use.
3. The role model can distinguish senior and junior capabilities.
4. Delegated runbooks are useful and understandable.
5. Approval workflows are not painful.
6. Audit events are useful during review.
7. The self-hosted base deployment is practical.
8. The architecture can become SaaS without a rewrite.

---

## 28. Build Plan Summary

The shortest credible path is:

1. Build the first execution/audit vertical slice.
2. Add real inventory and runner registration.
3. Harden execution handling.
4. Add role and policy enforcement.
5. Add delegated runbooks.
6. Add approvals.
7. Add enough TUI and web UX to make the product usable.
8. Harden for private beta.

Do not start with billing, provider integrations, terminal recording, compliance reports, or advanced policy-as-code.

The product becomes real when a junior engineer can safely run an approved task and a senior engineer can see exactly what happened afterwards.

