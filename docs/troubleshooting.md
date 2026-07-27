# Troubleshooting

Start with the health endpoint and the current identity. Most failures become easier to classify once you know whether the problem is startup, authentication, authorisation, queueing, runner access, or target execution.

```powershell
Invoke-RestMethod http://127.0.0.1:8080/api/v1/health
.\bin\vps.exe whoami
```

## The API will not start

Check the startup output for the selected tier and configuration validation errors. The default installation needs no PostgreSQL, S3, or NATS. If an external driver is selected, all required settings must be present.

Common causes include:

- The SQLite directory does not exist or is not writable.
- `ARTIFACT_ENCRYPTION_KEY` is missing or invalid.
- An external database or object-store URL is incomplete.
- Another process already owns the configured HTTP port.
- A migration cannot acquire the SQLite writer lock.

Do not fix an external backend error by silently changing the driver back to SQLite. Correct the configuration or deliberately select the self-contained tier.

## The CLI cannot connect

Confirm that the API is running and that `VPS_API_URL` points to the correct address. If the API is bound to localhost inside another machine or container, the CLI will not reach it from outside that network namespace.

```powershell
$env:VPS_API_URL = "http://127.0.0.1:8080"
.\bin\vps.exe whoami
```

## Authentication or organisation errors

Development mode can use the configured local identity headers. Shared deployments should use OIDC. Check the mapped subject, email, organisation, and role. A valid identity can still receive a denial when the requested server, runbook, or action is outside its scope.

Never solve an authorisation problem by giving the runner a more powerful identity. The API is the policy enforcement point.

## A job does not progress

Check that a runner is registered, healthy, and polling the same API. In local development, `SIMULATE=true` lets the runner complete the vertical slice without SSH credentials.

```powershell
.\bin\vps.exe runner list
.\bin\vps.exe exec list --limit 20
```

Inspect the job lease, retry count, last error, and runner logs. A crashed runner should release work after the lease expires. If it does not, run reconciliation and inspect the database queue state.

## Preflight fails

Preflight checks can fail because the runbook is not published, the target is unknown or unhealthy, required parameters are missing, approval is required, or the current identity lacks permission. Fix the specific preflight finding before retrying. Preflight is intended to prevent a predictable failure, not to be bypassed.

## Execution fails on the target

Separate control-plane failure from target failure. Confirm the runner reached the target, the SSH account has the expected permissions, the command timeout is appropriate, and the target has not changed since preflight. Review captured output and the exact runbook version.

Do not rerun a failed command blindly. First determine whether it was never started, partially completed, or completed but reported an error. Use an idempotent recovery runbook where possible.

## Output or artefacts are missing

Check the artefact directory, service-account permissions, encryption key, and manifest. A changed encryption key can make existing local artefacts unreadable. For S3, check object existence, checksum, bucket policy, lifecycle rules, and signed URL expiry.

## A schedule did not run

Confirm that the schedule is enabled, its timezone and next-run time are correct, the referenced runbook version is still published, and the embedded scheduler is running. Review the generated execution and audit events. A schedule should create an execution record even when preflight blocks the work.

## The web console is blank or shows no data

Check the browser's API base URL, API health, identity configuration, and browser network errors. A development identity header is not automatically available to a browser page. For shared access, configure OIDC and verify the callback URL.

## MCP tools are unavailable

From the `mcp` directory, run:

```powershell
npm run check
npm run smoke
```

Check that the AI client launches the server with an absolute path, that `VPS_API_URL` is reachable, and that the MCP process has the intended `VPS_USER` or OIDC settings. Read-only tools should work with writes disabled. Write tools should refuse calls without both the environment flag and `confirm=true`.

## Collecting a useful support report

Include the VPS Tools version, deployment tier, API health result, current identity, execution ID, runbook ID and version, target ID, runner ID, relevant timestamps, and redacted logs. Do not include credentials or unredacted command output.

