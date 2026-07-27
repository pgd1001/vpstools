# CLI and TUI reference

The binary is `vps`. All commands use the API configured by `--api-url`, `VPS_API_URL`, or the optional config file at `~/.config/vps-tools/config.yaml`.

## Common settings

```powershell
$env:VPS_API_URL = "http://localhost:8080"
$env:VPS_USER = "user_junior"
.\bin\vps.exe whoami
```

The flag takes precedence over the environment and config file. Most list and detail commands support `--output json`, which is the preferred format for scripts.

For production access, use a short-lived bearer token:

```powershell
$env:VPS_API_TOKEN = "replace-with-a-short-lived-token"
.\bin\vps.exe whoami
```

Privileged operators can create one with `vps auth create-token --name operator-cli`. The token is displayed once.

## Identity

```text
vps whoami
```

## Production preflight

Run the doctor command before a change or after an installation. It checks API reachability, database and encrypted artefact readiness, and the authenticated operator identity. It returns exit code 1 if any check fails.

```text
vps doctor
vps doctor --json
```

Use `--json` in deployment scripts and health gates. A failed identity check normally means that `VPS_API_TOKEN` is missing, expired, or not permitted for the selected organisation.

## Server inventory

```text
vps server add <name> [--hostname host] [--environment development|staging|production]
  [--ssh-port 22] [--ssh-user root] [--provider provider]
  [--tags '[{"key":"role","value":"web"}]'] [--output table|json]
vps server list [--environment production] [--tag-key role] [--tag-value web] [--output table|json]
vps server inspect <server> [--output table|json]
vps server check <server> [--output table|json]
```

Use `server:<id|name>` in later commands when you want to make the target type explicit.

## Runners

```text
vps runner list [--output table|json]
vps runner register <name> [--version v] [--hostname host] [--platform linux|darwin|windows]
  [--ip-address address] [--type customer_managed] [--output table|json]
vps runner registration-token [--runner-id rnr_...] [--output table|json]
```

Registration tokens are credentials. Keep them out of shell history, chat transcripts, and CI logs.
When a runner ID is supplied, the token is bound to that runner. Issuing a replacement revokes the previous active token. Tokens expire after one hour.

## Automation emergency control

```text
vps automation status [--output table|json]
vps automation list [--output table|json]
vps automation create --name NAME --runbook RUNBOOK --target TARGET --reason REASON [--params name=value] [--interval 3600]
vps automation pause [--reason "incident response"] [--output table|json]
vps automation resume [--output table|json]
vps automation disable <schedule-id> --confirm [--output table|json]
```

Pausing is organisation-wide and stops new scheduled runs. It does not cancel executions that are already queued. The pause and resume actions require a senior engineer or above and are recorded in the audit trail.

## Direct execution

Direct execution is intended for senior or administrator workflows. Junior engineers should use published runbooks.

```text
vps exec <target> [--reason "why"] [--idempotency-key change-123] [--wait] [--timeout 300] -- <command>
vps exec <target> -- <command> --dry-run
vps exec status <execution-id> [--output table|json]
vps exec list [--status queued] [--limit 20] [--mine] [--delegated] [--output table|json]
vps exec cancel <execution-id> [--output table|json]
```

The `--` separator is required when the command contains flags that Cobra could interpret as VPS Tools flags. `--dry-run` only previews the CLI request. It does not call the API preflight endpoint. Use a stable `--idempotency-key` when a script may retry after a network timeout.

## Runbooks

```text
vps runbook list [--output table|json]
vps runbook inspect <runbook> [--output table|json]
vps runbook create <name> [--file definition.yml] [--title title]
  [--command command] [--description text] [--risk low|medium|high|critical]
  [--environment development|staging|production] [--output table|json]
vps runbook publish <runbook>
vps runbook run <runbook> --target <target> [--reason text]
  [--params 'key=value,key=value'] [--idempotency-key change-123] [--wait] [--timeout 300] [--output table|json]
```

Use `--params` only for declared parameters. The API validates the final values again.

## Approvals and audit

```text
vps approvals list [--status pending|approved|denied] [--output table|json]
vps approvals approve <approval-id> [--note "reviewed"] [--output table|json]
vps approvals deny <approval-id> --note "reason" [--output table|json]
vps audit search [--actor actor-id] [--limit 20]
vps audit verify
```

Always inspect the approval brief before approving. A denial note is mandatory.

## TUI

```text
vps tui
```

| Key | View |
|---|---|
| `1` | Servers |
| `2` | Runbooks and task search |
| `3` | Executions and detail |
| `4` | Approvals |
| `5` | Schedules |
| `6` | Audit |
| `r` | Refresh |
| `h` | Help |
| `q` | Back or quit |

The TUI schedule view is read-only. Create and disable schedules from the web console, API, or MCP tools.
