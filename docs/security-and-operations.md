# Security and operations

VPS Tools is designed around a control-plane trust boundary. The API authorises work, the runner executes an already-authorised job, and the audit trail records privileged actions. Operators should preserve that separation in every deployment.

## Deployment checklist

For a self-contained installation:

- Run the API and runner as a dedicated service account.
- Keep `svrtools.db`, the artefact directory, and encryption key outside the source checkout.
- Set restrictive permissions on the database, artefacts, logs, and backup directory.
- Use a stable `VPS_ARTIFACT_ENCRYPTION_KEY` and store it outside the repository.
- Bind the API to localhost when only local clients need access. Put it behind TLS when accessed over a network.
- Enable OIDC for shared environments. Development header identity is not an authentication system.
- Schedule and test backups of both SQLite metadata and artefacts.

For an extended installation, add PostgreSQL, S3-compatible storage, and JetStream only when the workload or availability requirements justify them. Validate each external backend at startup and fail clearly if it is incomplete.

## Identity and authorisation

Every operator should have a distinct identity. Use OIDC for production users and map identity claims to the organisation and role expected by the API. Keep the control plane as the only place where permissions are evaluated.

The runner must not decide whether a user may execute a command. It should validate the signed or leased job, enforce its execution scope, run the exact authorised command, redact sensitive output, and report the result.

Use the smallest role that supports the task:

- Viewers can inspect inventory, runbooks, executions, schedules, and audit data.
- Operators can perform approved operational work within their assigned scope.
- Senior engineers can draft and publish runbooks, review approvals, and manage higher-risk work.
- Administrators manage identity, organisations, runners, policy, retention, and deployment configuration.

## Secrets and sensitive output

Do not put passwords, private keys, tokens, or complete connection strings in runbook text, CLI arguments, prompts, audit messages, or issue reports. Pass references to a secret source where supported, and redact output before it reaches the API or artefact store.

Local artefacts use atomic writes, content hashes, restricted permissions, retention controls, and encryption at rest when configured. S3 deployments should use encrypted objects, checksums, lifecycle rules, and short-lived signed URLs.

## Backups and restore

The local backup command packages SQLite metadata, encrypted artefacts, and a manifest. A backup is useful only if it can be restored, so test the complete process regularly.

```powershell
.\bin\vps.exe backup create --output .\backups\nightly
.\bin\vps.exe backup verify --input .\backups\nightly
```

Keep backup retention separate from artefact retention. Protect the encryption key independently from the backup files. A database backup without referenced artefacts is incomplete, and artefacts without their metadata are difficult to interpret.

## Queue and runner reliability

The database queue uses leases, retries, idempotency keys, and dead-letter states. If a runner stops while holding a lease, reconciliation should make the work claimable after the lease expires.

When diagnosing a stuck execution, record:

- Execution and job IDs.
- Target server and runner ID.
- Lease owner and lease expiry.
- Retry count and last error.
- Whether the runner can reach the API and target.
- Whether the command has a safe timeout and rollback.

JetStream is at-least-once delivery. Consumers must use durable pull consumers, explicit acknowledgements, bounded redelivery, and idempotent handlers. Duplicate delivery must never result in a second infrastructure change.

## Audit and incident response

Search the audit trail before changing a failed system. Record who requested the work, who approved it, which runbook version ran, which target was used, what the runner reported, and what artefacts were created.

For a suspected unsafe execution:

1. Disable the affected schedule or runner if it is safe to do so.
2. Preserve the execution output, audit records, and relevant configuration.
3. Identify the exact runbook version and approval chain.
4. Check for duplicate jobs or redelivery.
5. Revoke or rotate exposed credentials.
6. Restore service using a reviewed runbook.
7. Document the incident and update the runbook or policy before re-enabling automation.

## Production readiness review

Before a shared production rollout, verify OIDC login, tenant isolation, role checks, runner scope checks, secret redaction, immutable runbook versions, audit completeness, backup restore, lease recovery, and failure handling. The same API, CLI, AI, security, and audit tests should pass regardless of the selected storage and queue tier.

