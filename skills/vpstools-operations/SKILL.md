---
name: vpstools-operations
description: Operate VPS Tools through its MCP server for infrastructure inventory, runbook discovery, preflight checks, approvals, execution monitoring, schedules, and audit investigation. Use when an AI agent needs to inspect or safely request operational work through VPS Tools.
---

# VPS Tools operations

Use the VPS Tools MCP server as the control plane for infrastructure work. Treat the CLI, web console, and MCP tools as equivalent interfaces to the same policy and audit system.

## Operating sequence

1. Call `vps_whoami` and `vps_health` before acting.
2. Discover the relevant runbook with `vps_list_runbooks` and `vps_get_runbook`.
3. Inspect target scope with `vps_list_servers`. Do not guess server IDs or environments.
4. Call `vps_preflight_runbook`. It is read-only and never queues work.
5. Explain the target, environment, risk, parameters, expected effect, and approval state to the user.
6. Request explicit confirmation before calling `vps_execute_runbook` with `confirm=true`.
7. For an approval response, inspect `vps_get_approval` first. Never approve based only on an ID or a short summary.
8. After execution, poll `vps_get_execution` until a terminal state and report per-target results.
9. Use `vps_search_audit` to provide an evidence trail when the user asks what happened.

## Safety rules

- Never invent a runbook, target, parameter, approval, or execution ID.
- Never use a raw shell command when a published runbook exists.
- Never skip preflight.
- Never pass `confirm=true` without explicit user confirmation in the current task.
- Treat production and high or critical risk actions as approval-sensitive.
- Do not suggest that a successful queue response means the operation succeeded. Check execution state.
- Preserve the exact runbook version, target snapshot, reason, and approval state in summaries.
- Do not reveal secrets or reproduce unredacted output. Link results to the execution and audit evidence instead.
- If a tool reports that writes are disabled, stop and ask an administrator to review `VPS_MCP_ALLOW_WRITES`.

## Automation schedules

Use `vps_list_schedules` to inspect existing schedules. Creating or disabling one requires explicit confirmation and a senior-authorized MCP identity. Schedules are fixed-interval in the self-contained tier. High and critical risk runbooks are not executed unattended.

## Failure handling

- Authentication error: report the identity configuration problem and do not retry with another user.
- Preflight failure: explain the returned policy or validation reason and ask for a permitted correction.
- Approval required: provide the approval ID and brief, then wait for a human decision.
- Partial execution: report each target separately and recommend the documented recovery path.
- Runner or API failure: check health, execution state, and audit events before proposing a retry.
