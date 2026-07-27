# VPS Tools MCP server

This local stdio server exposes safe VPS Tools operations to MCP-compatible AI clients and agents.

## Defaults and safety

Read tools are available by default. Write tools are disabled unless both conditions are met:

1. `VPS_MCP_ALLOW_WRITES=true` is set in the MCP process environment.
2. The tool call includes `confirm=true` after the user has explicitly confirmed the change.

Execution requests always run preflight first. High and critical risk runbooks follow the normal approval path. The MCP server never exposes raw shell execution as a separate tool.

## Tools

Read-only tools cover health, identity, servers, runbooks, preflight, approvals, executions, audit search, schedules, and automation state. Controlled-write tools cover runbook execution requests, approval decisions, schedule creation or disabling, and the organisation-wide automation pause or resume.

The server currently provides:

```text
vps_health                 vps_whoami              vps_list_servers
vps_list_runbooks          vps_get_runbook         vps_preflight_runbook
vps_execute_runbook        vps_list_approvals      vps_get_approval
vps_approve_request        vps_deny_request        vps_list_executions
vps_get_execution          vps_cancel_execution    vps_search_audit
vps_verify_audit
vps_list_schedules
vps_automation_status      vps_pause_automation    vps_resume_automation
vps_create_schedule        vps_disable_schedule
```

## Configuration

```text
VPS_API_URL=http://localhost:8080
VPS_USER=user_senior
VPS_API_TOKEN=
VPS_MCP_ALLOW_WRITES=false
```

For production, set `VPS_API_TOKEN` to an expiring bearer token. The server sends `Authorization: Bearer` and does not send the development identity header when a token is configured. For local development, set `VPS_USER` to the intended provisioned identity. The server does not silently choose a senior identity when the variable is absent.

For an OIDC-backed deployment, provide `VPS_WEB_SHARED_SECRET`, `VPS_OIDC_SUBJECT`, and `VPS_OIDC_EMAIL`. The API must be configured to accept the matching internal identity headers.

## Run locally

```bash
npm install
npm run check
npm start
```

Configure an MCP client with:

```json
{
  "mcpServers": {
    "svrtools": {
      "command": "npm",
      "args": ["--prefix", "mcp", "run", "start"],
      "env": {
        "VPS_API_URL": "http://localhost:8080",
        "VPS_API_TOKEN": "replace-with-a-short-lived-token",
        "VPS_MCP_ALLOW_WRITES": "false"
      }
    }
  }
}
```

For a production installation, use an absolute path or a packaged executable and keep the server on a trusted host. Do not expose the stdio process to an untrusted network.
