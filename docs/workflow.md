# Operational workflow

VPS Tools separates the human task from the shell command that performs it. A runbook is versioned and published by a senior engineer. An operator selects a permitted target and supplies only the declared parameters.

## Lifecycle

```text
Draft -> Published -> Preflight -> Requested -> Awaiting approval -> Approved
  -> Queued -> Running -> Verifying -> Succeeded
```

Failure and recovery states include `failed`, `partially_succeeded`, `cancelled`, `expired`, and `blocked`, depending on the path taken.

## What preflight checks

Preflight is read-only. It validates:

- The runbook exists and is published.
- The current actor is allowed to use it.
- Target identifiers resolve inside the actor's organisation.
- Targets do not mix environments in one request.
- The runbook allows the selected environment.
- Parameter names, types, required values, defaults, and allowed values are valid.
- The target count and runbook target constraints are acceptable.
- A production reason exists when required.
- The proposed command preview is redacted.

Use preflight before asking for an approval or user confirmation. A successful preflight means the request is valid, not that it has changed a server.

## Target formats

- `server:<id>` or `server:<name>` selects one server.
- `tag:<key>=<value>` selects all matching servers.
- A plain server name is also accepted by the API.

Do not combine development, staging, and production targets in one operation. Split them into separate requests with separate reasons and approvals.

## Runbook parameters

Runbook parameters are declared in YAML and are rendered with shell-safe quoting. Example:

```yaml
spec:
  parameters:
    - name: service
      type: string
      required: true
      allowedValues: [nginx, ssh]
  execution:
    command: systemctl restart ${service}
```

Unknown parameters, missing required values, invalid numeric values, and values outside `allowedValues` are rejected before queueing. Do not place secrets in parameters or command previews.

## Approvals

High-risk production work normally creates an approval request. An approver should inspect the complete brief, including runbook version, target snapshot, environment, parameters, risk, reason, expiry, rollback plan, and verification or evidence plan when the runbook declares them. Denials require a note. Approval actions are audited and must follow the organisation's separation-of-duties policy.

In the TUI, select a permitted runbook and press `x` to open the guided task form. Use `Tab` to move between target, reason, and parameter fields. Press `p` for read-only preflight, then `Enter` to submit. The web console exposes the same task inputs and preflight action.

## Execution and evidence

An execution has one overall state and one state per target. The runner uses a lease so work can be reclaimed after an interrupted runner. A failed attempt is requeued while its target has attempts remaining. The first retry waits one second, then the delay doubles up to one minute. An expired lease or failed attempt at the attempt limit moves the target to `dead_letter` and the execution to `failed`. Output is redacted at the API boundary and large output is stored as encrypted artefacts. Use the execution detail endpoint or CLI status command to see the timeline and target results.

## Safe operator response

When a task fails, report the execution ID, affected targets, terminal state, error summary, and the next permitted recovery action. Do not immediately repeat a production operation just because a request timed out. First check whether the runner completed it and inspect the audit event.
