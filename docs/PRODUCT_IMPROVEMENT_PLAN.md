# VPS Tools Product Improvement Plan

**Status:** Proposed backlog
**Owner:** Product and engineering
**Last updated:** 26 July 2026
**Related documents:** [Operator Guide](operator-guide/README.md), [Known Limitations](KNOWN_LIMITATIONS.md), [MVP Build Plan](../vps_tools_mvp_build_plan.md)

## 1. Purpose

VPS Tools is intended to give infrastructure teams a safe, repeatable way to delegate operational work.

Senior engineers define and review runbooks. Junior engineers use approved tasks to perform day-to-day operations. The product applies policy, approval, automation, audit, and AI assistance around that workflow.

This document turns the product review into a tracked improvement backlog. It records the gaps to close, the outcomes expected from each workstream, and the acceptance tests that define progress.

The guiding product principle is:

> A junior engineer should be able to complete a permitted operational task without needing to understand shell syntax, queue internals, or hidden policy rules.

## 2. Current assessment

The current application has a useful governance foundation:

- Server inventory and runner management
- Controlled execution through the API and runner
- Versioned runbooks
- Role-based authorisation
- Approval requests
- Audit events
- CLI, TUI, and web console access
- Self-contained SQLite and local artefact deployment
- Optional PostgreSQL, S3-compatible storage, and JetStream extensions

The main product gap is that the system still exposes much of its internal execution model directly to users. It needs a guided task layer, clearer progress feedback, stronger runbook safety, and better parity between the CLI, TUI, and web console.

## 3. Priority model

- **P0:** Safety, correctness, or trust issue. Address before wider use.
- **P1:** Core workflow gap that limits junior engineer adoption or creates avoidable operational risk.
- **P2:** Important product improvement for efficiency, clarity, and scale.
- **P3:** Longer-term enterprise or differentiation work.

## 4. Product workstreams

## 4.1 Implementation status

The first implementation slice is complete and verified. These items are now implemented:

- Runbook parameters have type, required, default, allowed-value, and unknown-parameter validation.
- Exact runbook substitutions are shell-quoted before execution.
- Approval and execution records preserve target snapshots.
- Mixed-environment target selections are rejected.
- Approval-to-execution creation is transactional.
- Sensitive command previews are redacted while runners retain the protected execution command separately.
- Runbook preflight mode reports target scope, risk, approval requirement, and a redacted command preview without queueing work.
- Execution timeline events are persisted and returned by the execution detail API.
- Web forms use immutable state updates, structured API errors, confirmation for destructive actions, and a guided run task flow.
- CLI approval denials require a reason, and TUI approval actions require confirmation.
- TUI audit navigation and small-terminal sizing have been corrected.
- Interval-based schedules are available through the API and embedded scheduler in the self-contained tier. Scheduled executions use an explicit automation identity, target snapshots, audit events, and the normal runbook policy checks.
- High and critical risk runbooks are blocked from unattended scheduling until an approval workflow is connected.
- A vendor-neutral AI provider interface now carries redacted prompts, evidence, responses, and usage metadata.
- Full Go tests, `go vet`, and the web production build pass.

The remaining items in this document are still planned unless explicitly marked here or in a future release note. The schedule and AI changes are foundations, not complete end-user features.

### P0-A. Safe runbook execution

**Outcome:** A runbook cannot execute unsafe or ambiguous input, and an approval decision describes exactly what will happen.

**Work items**

- Validate parameter types, required values, defaults, and allowed values.
- Replace raw command substitution with shell-safe rendering or structured command arguments.
- Store an immutable target snapshot in every approval request and execution.
- Reject or split requests containing mixed environments.
- Enforce maximum target count and concurrency limits.
- Make approval creation, execution creation, and audit persistence transactionally consistent.
- Apply the runbook approval role and policy settings consistently.
- Add explicit expiry and conflict handling for approvals.
- Prevent self-approval where separation of duties requires it.

**Acceptance criteria**

- Invalid, missing, and disallowed parameters fail before a job is queued.
- Parameter values cannot alter command structure.
- An approver can see the exact runbook version, targets, parameters, environment, risk, and rollback information.
- A mixed development and production request is never evaluated as one environment.
- Failed database writes cannot leave an approval or audit record in a misleading state.
- Security tests cover command injection, target tampering, approval conflicts, and cross-organisation access.

### P1-A. Guided junior engineer task workflow

**Outcome:** Junior engineers complete standard tasks through a clear, role-aware workflow rather than by assembling commands.

Introduce a task layer above technical runbooks. A task should describe the intended outcome and expose only the inputs that a permitted operator needs.

**Task model**

- Human-friendly title and outcome
- Required inputs and validation rules
- Permitted target types and environments
- Risk and expected duration
- Pre-checks
- Execution steps
- Post-checks
- Rollback or recovery procedure
- Evidence to collect
- Approval requirements
- Escalation instructions
- Related documentation

**Work items**

- Add a role-aware task inbox.
- Show assigned, ready, awaiting approval, running, blocked, failed, and completed work.
- Add guided target selection and parameter forms.
- Add preflight and dry-run results before submission.
- Explain why approval is required and who can provide it.
- Add clear next actions for blocked and failed tasks.
- Add task assignment, acknowledgement, due dates, and escalation.

**Acceptance criteria**

- A junior engineer can complete a common task without writing shell syntax.
- Every task shows its owner, target scope, current state, and next action.
- A blocked task explains the reason and the permitted resolution path.
- The same task behaves consistently through CLI, TUI, and web console.
- A task cannot expose controls that the current user is not allowed to use.

### P1-B. Execution lifecycle and recovery

**Outcome:** Operators can understand, control, and recover work from request through verification.

Use a common lifecycle across API, CLI, TUI, and web console:

`Draft -> Requested -> Awaiting approval -> Approved -> Queued -> Running -> Verifying -> Succeeded`

Failure states should include `Failed`, `Partially succeeded`, `Cancelled`, `Expired`, and `Blocked`.

**Work items**

- Add a live execution timeline.
- Show per-target progress and results.
- Add cancel for queued work.
- Add retry for failed targets only.
- Add safe re-run using the same published version.
- Add escalation and operator notes.
- Add evidence links for output, diagnostics, and verification.
- Add idempotency and duplicate-request protection.
- Add automatic recovery for abandoned leases.

**Acceptance criteria**

- An operator can identify the current execution state without reading raw logs.
- Partial success is represented accurately.
- A runner failure does not silently lose work or cause duplicate execution.
- Retry and re-run behaviour is explicit and auditable.
- Every state transition records an actor and timestamp.

### P1-C. Approval briefs and delegated work

**Outcome:** Approvers can make a decision from one complete, understandable view.

**Approval brief contents**

- Requester and delegating senior engineer
- Exact runbook version
- Target snapshot and environment
- Parameters and proposed action
- Risk rating and blast radius
- Pre-check results
- Rollback plan
- Expected duration
- Expiry time
- Previous related executions
- Audit history

**Work items**

- Add approval detail views in the TUI and web console.
- Require confirmation before approval or denial.
- Capture a reason for denial or requested changes.
- Support request changes and escalation.
- Add configurable approval expiry.
- Add multi-level approval for selected policies.
- Notify requesters and approvers when the state changes.

**Acceptance criteria**

- An approver does not need a second system to understand the request.
- Approve, deny, request-change, and escalate actions are distinct.
- Denials always include an explanation.
- Approval state changes appear in the requester’s task view.

### P1-D. Runbook authoring and quality controls

**Outcome:** Senior engineers can create safe, tested, documented operational procedures.

**Work items**

- Add a multi-step runbook editor.
- Add schema validation while editing.
- Add version diff and change history.
- Add test and simulation execution.
- Add pre-check, post-check, and rollback sections.
- Add target constraints, concurrency, maintenance windows, and approval rules.
- Add expected output and troubleshooting guidance.
- Add publish review and disable controls.
- Add Git import and export as a later extension.

**Acceptance criteria**

- A runbook cannot be published while required safety fields are invalid or missing.
- A reviewer can compare the proposed version with the previous published version.
- A senior engineer can test a runbook without changing production state.
- Published executions always reference an immutable version.
- Junior-facing tasks hide technical fields that do not help the operator complete the work.

### P1-E. TUI usability and feedback

**Outcome:** The TUI is dependable during real operational work and gives immediate, understandable feedback.

**Known issues to address**

- Audit mode currently consumes navigation and quit key presses.
- Network requests are handled synchronously and can make the interface appear frozen.
- Several API errors are ignored.
- Loading, empty, retry, and last-refreshed states are inconsistent.
- Runbook selection does not yet open a useful parameter or detail workflow.
- Approval actions need confirmation and decision notes.
- Execution output needs paging, redaction, truncation, and target-level status.
- Small terminal dimensions need safer layout handling.

**Work items**

- Move network work into asynchronous Bubble Tea commands and messages.
- Add a shared loading, error, empty, and success state model.
- Add a task inbox as the default operational view.
- Add guided runbook parameters and preflight review.
- Add approval detail and confirmation screens.
- Add execution timeline, retry, cancel, escalation, and output paging.
- Keep the keyboard map consistent across screens.
- Generate or validate help text as part of the release process.

**Acceptance criteria**

- Every screen has a visible response while data is loading.
- Failed requests show a useful message and a retry action.
- All destructive actions require confirmation.
- The TUI remains usable in a small terminal window.
- Keyboard users can reach every available action.
- TUI behaviour matches the web console for the core task lifecycle.

### P1-F. Web console usability and accessibility

**Outcome:** The web console is clear, responsive, accessible, and consistent across roles.

**Work items**

- Split the single page component into feature components and typed hooks.
- Fix immutable form state updates.
- Add typed API error handling and correlation identifiers.
- Add shared status badges, alerts, empty states, confirmation dialogs, tables, and timelines.
- Add proper labels, field IDs, focus states, keyboard actions, and semantic controls.
- Make table actions accessible without relying on row clicks.
- Add responsive layouts for smaller screens.
- Add role-aware home pages and navigation.
- Add task inbox, approval brief, runbook editor, and live execution views.
- Add loading, retry, success, and persistent service-status feedback.
- Add browser tests for critical workflows.

**Acceptance criteria**

- Forms identify invalid fields and preserve entered values after an error.
- Buttons show disabled or pending states while a request is in progress.
- Table actions work with keyboard and assistive technology.
- A user always knows whether data is loading, current, stale, or failed.
- The web console and API expose the same core capabilities as the CLI.
- Critical workflows have automated browser coverage.

### P2-A. Deterministic automation

**Outcome:** Repetitive operational work runs safely with clear policy boundaries and full auditability.

**Work items**

- Add scheduled runbooks. **Initial interval-based API and embedded scheduler implemented.**
- Add event-triggered diagnostics.
- Add maintenance windows.
- Add health checks before execution.
- Add post-execution verification.
- Add retry for transient failures.
- Add rollback when verification fails.
- Add notification and escalation rules.
- Add deduplication and idempotency controls.
- Enforce target and concurrency limits.
- Add a global automation pause or kill switch.

**Acceptance criteria**

- Automated actions use the same policy and approval checks as manual actions.
- Each automated action records an explicit automation identity.
- Duplicate events do not create duplicate executions.
- Failed verification produces a visible recovery path.
- Operators can pause future automation without deleting configuration.

### P2-B. AI-assisted operations

**Outcome:** AI reduces investigation and documentation effort without making unaudited operational decisions.

**Initial capabilities**

- Read-only operations assistant
- Runbook recommendation
- Diagnostic and failure summaries
- Approval brief generation
- Post-execution summaries
- Senior runbook authoring assistance
- Anomaly detection over execution history

**Guardrails**

- AI suggestions never bypass authorisation or approval.
- AI-generated actions remain drafts until a human submits them.
- Responses cite the underlying runbook, execution, audit, or artefact evidence.
- Prompt and response retention follows configured privacy rules.
- Local models remain available for sensitive deployments.
- Prompt injection and untrusted output handling are tested explicitly.

**Implementation note:** The provider and redaction boundary is now present in `packages/ai`. Model configuration, evidence retrieval, AI endpoints, local-model adapters, and human-reviewed draft workflows remain to be built.

**Acceptance criteria**

- AI answers identify their evidence sources.
- AI cannot execute a command directly without the standard task and policy path.
- Operators can see whether a recommendation came from a human, rule, or model.
- Sensitive data is redacted according to the same storage and retention rules as other output.
- AI features work with local and extended storage backends.

### P2-C. Documentation and product guidance

**Outcome:** Users can learn the product by role and recover from common situations without informal support.

**Work items**

- Add a junior engineer daily workflow guide.
- Add a senior runbook authoring guide.
- Add an approver decision guide.
- Add an administrator deployment and policy guide.
- Add failure recovery and incident procedures.
- Add automation configuration guidance.
- Add AI privacy and model configuration guidance.
- Document the task lifecycle and status meanings.
- Add contextual help links in the TUI and web console.
- Keep known limitations aligned with actual product behaviour.
- Correct examples where documentation syntax does not match implementation.

**Acceptance criteria**

- Each supported role has a complete first-task guide.
- Documentation examples are tested against the current CLI and API.
- TUI, web, and operator documentation use the same names for states and actions.
- Known limitations are reviewed as part of each release.

## 5. Delivery roadmap

### Phase 1, safety and trust

- Complete P0-A.
- Add approval detail and immutable target snapshots.
- Fix high-risk CLI, TUI, and web feedback issues.
- Add security and state-transition tests.

### Phase 2, junior workflow

- Complete P1-A, P1-B, and P1-C.
- Introduce the task model and task inbox.
- Add guided execution and live progress.

### Phase 3, senior authoring and deterministic automation

- Complete P1-D and P2-A.
- Add testing, verification, rollback, schedules, and notifications.

### Phase 4, AI assistance

- Complete P2-B.
- Start with evidence-linked, read-only and draft-producing features.

### Phase 5, enterprise maturity

- Custom roles and relationship-based permissions
- Policy simulation and policy-as-code integration
- Incident management integrations
- Session recording
- Compliance reporting
- Advanced multi-organisation administration

## 6. Cross-interface parity matrix

The following core actions should be available with equivalent behaviour in every interface.

| Capability | CLI | TUI | Web console | API/SDK |
|---|---:|---:|---:|---:|
| Browse available tasks | Yes | Yes | Yes | Yes |
| Validate parameters | Yes | Yes | Yes | Yes |
| Run preflight checks | Yes | Yes | Yes | Yes |
| Submit for approval | Yes | Yes | Yes | Yes |
| Review approval brief | Yes | Yes | Yes | Yes |
| Approve or deny with reason | Yes | Yes | Yes | Yes |
| Monitor live execution | Yes | Yes | Yes | Yes |
| Retry failed targets | Yes | Yes | Yes | Yes |
| View evidence and audit history | Yes | Yes | Yes | Yes |
| Run AI-assisted diagnosis | Yes | Yes | Yes | Yes |

Any exception should be documented and treated as a temporary limitation.

## 7. Persona acceptance tests

### Junior engineer

- Can find a permitted task quickly.
- Can understand the expected outcome and risk.
- Can submit valid parameters without shell knowledge.
- Can see approval status and execution progress.
- Can recover from a failure or escalate it.

### Senior engineer

- Can draft, test, review, publish, assign, disable, and version a runbook.
- Can see which tasks junior engineers can perform.
- Can inspect failures and improve a runbook using evidence.

### Approver

- Can make a decision from a complete approval brief.
- Can see exact targets, parameters, risk, and rollback information.
- Can approve, deny, request changes, or escalate with a recorded reason.

### Administrator

- Can configure identity, policies, storage, queues, automation, and AI providers.
- Can see service health and deployment tier.
- Can pause automation and recover from a failed dependency.

### Auditor

- Can reconstruct who requested, approved, executed, and reviewed an action.
- Can connect an execution to its runbook version, targets, outputs, artefacts, and audit events.
- Can export evidence without granting execution permissions.

## 8. Tracking rules

Each work item should become an issue or implementation task with:

- One priority from P0 to P3
- A named owner
- A linked acceptance test
- The affected interfaces
- Security and audit impact
- Migration or compatibility notes
- Documentation updates required

A phase is complete only when its acceptance criteria pass in the self-contained deployment tier and the extended deployment tier where the feature applies.
