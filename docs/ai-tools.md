# AI tools and agent operations

VPS Tools can be operated by an AI assistant through the bundled Model Context Protocol (MCP) server. The server gives an assistant structured access to inventory, runbooks, approvals, executions, schedules, automation pause state, and audit history. It does not give an assistant an unrestricted shell.

## How the integration works

```text
AI assistant or agent
        |
        | MCP over local stdio
        v
VPS Tools MCP server
        |
        | HTTP API with the configured identity
        v
VPS Tools control plane
        |
        +--> policy and approval checks
        +--> database-backed queue
        +--> runner and audit trail
```

The MCP server is a client of the VPS Tools API. The API remains the policy enforcement point, so an assistant cannot bypass role checks, runbook permissions, approvals, or runner controls.

## Install and configure the MCP server

From the repository root:

```powershell
Set-Location mcp
npm install
npm run check
```

Configure the connection for the AI client that will launch the server. A typical local configuration is:

```json
{
  "mcpServers": {
    "vpstools": {
      "command": "npm",
      "args": ["--prefix", "C:/path/to/vps-tools-new/mcp", "run", "start"],
      "env": {
        "VPS_API_URL": "http://127.0.0.1:8080",
        "VPS_USER": "engineer@example.com",
        "VPS_API_TOKEN": "",
        "VPS_MCP_ALLOW_WRITES": "false"
      }
    }
  }
}
```

Use an absolute path in the client configuration. On Windows, forward slashes avoid escaping problems in JSON. The API must already be running.

For production CLI, SDK, or MCP access, use an expiring `VPS_API_TOKEN` rather than `VPS_USER`. For a shared or OIDC-protected API, configure the authentication variables described in [the MCP README](../mcp/README.md). Never place a production secret in a checked-in client configuration.

## Available tool groups

| Group | What the assistant can do |
| --- | --- |
| Context | Check health and identify the current user and organisation |
| Inventory | List servers and their status |
| Runbooks | List, inspect, and preflight runbooks |
| Executions | Start approved work and inspect execution state |
| Approvals | Review, approve, or deny requests when writes are enabled |
| Audit | Search the append-only audit trail |
| Schedules | Review, create, and disable recurring work |
| Automation control | Inspect or pause/resume new scheduled work during an incident |

The exact tool names are prefixed with `vps_`, for example `vps_list_servers`, `vps_preflight_runbook`, and `vps_search_audit`.

## Safe operating mode

Start with read-only access. With `VPS_MCP_ALLOW_WRITES=false`, the server exposes inspection and planning tools without permitting changes. This is the right setting for discovery, reporting, incident triage, and assistant evaluation.

The self-contained smoke test can run a live MCP check against the temporary API by setting `VPS_MCP_SMOKE_LIVE=true`. It passes the same API token environment to the stdio child process and verifies health, identity, and automation state through MCP.

Write operations require both conditions below:

1. `VPS_MCP_ALLOW_WRITES=true` is set for the MCP process.
2. The tool call includes `confirm=true`.

The confirmation flag is an explicit acknowledgement by the calling agent. It does not replace API authorisation or a required human approval. An assistant should ask for confirmation immediately before any action that changes infrastructure, schedules work, approves a request, or changes a request state.

## Recommended agent sequence

An operations agent should follow this sequence for any potentially disruptive task:

1. Call `vps_health` and `vps_whoami`.
2. Inspect the target with `vps_list_servers`.
3. Find the published runbook with `vps_list_runbooks` and `vps_get_runbook`.
4. Run `vps_preflight_runbook` with the target and parameters.
5. Explain the proposed action, expected impact, timeout, and rollback.
6. Check `vps_list_approvals` when the runbook requires approval.
7. Ask the user for explicit confirmation before writing.
8. Call `vps_execute_runbook` with `confirm=true` only after confirmation and any approval are present.
9. Poll `vps_get_execution` until the execution reaches a terminal state.
10. Search `vps_search_audit` and report the execution ID, result, and evidence.

The same sequence works with a local model or a managed model. The model provider changes the conversation layer, not the control plane contract.

## Example prompts

Read-only inventory review:

> Check the health of VPS Tools, identify my role, and list servers that are currently unhealthy. Do not change anything.

Runbook planning:

> Find the published runbook for checking disk usage on `server:prod-web-01`. Show me the preflight result, expected commands, and any approval requirement. Do not execute it.

Controlled execution:

> Preflight the `restart-nginx` runbook on `server:staging-web-01` with reason `staging validation`. Tell me the risk and rollback first. I will confirm separately if it is safe to run.

Audit reporting:

> Show all failed executions and related audit events from the last 24 hours. Group them by server and include the runbook version.

Schedule review:

> List all enabled schedules. Flag any schedule that runs against production or has no recent successful execution. Do not modify schedules.

## Prompt and evidence handling

AI requests can contain operational data, command output, hostnames, and incident details. Keep prompts free of credentials and private keys. Use runbook parameters for references to secrets rather than pasting secret values into the conversation.

The self-contained tier records AI request metadata and redacted content according to retention settings. Small evidence objects remain in the local artefact store. Customers with stricter data boundaries can use a local model and keep the API, runner, database, and artefacts inside their own environment.

Assistants should return evidence, not just a conclusion. A useful result includes the execution ID, target, runbook version, state transition, relevant output, audit event ID, and a clear statement when no change was made.

## Automation patterns for agents

Good candidates for agent assistance include:

- Daily read-only health summaries.
- Preflight reports for planned maintenance.
- Failed execution triage with links to output and audit history.
- Approval summaries for a senior engineer.
- Detection of stale runners, expired certificates, low disk space, or missed schedules.
- Drafting a new runbook from an incident transcript, followed by human review and publication.

Keep autonomous writes narrow. A useful policy is read-only by default, automatic execution only for published low-risk runbooks, and human approval for production changes, destructive actions, credential changes, and broad target groups.

## Limitations and production guidance

The current MCP server is local and stdio-based. It is suitable for a desktop AI client or a locally hosted agent. A remote multi-tenant MCP gateway, per-tool policy scopes, streaming execution updates, and richer model-provider controls are future work.

Treat the MCP process as an operator client. Give it the minimum API identity it needs, keep write access disabled unless required, and retain the normal audit and approval workflow.

