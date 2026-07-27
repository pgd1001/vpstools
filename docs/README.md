# VPS Tools documentation

This is the documentation hub for VPS Tools, a controlled operations platform for infrastructure teams.

## Start here

| Need | Read |
|---|---|
| Install a self-contained instance | [Getting started](getting-started.md) |
| Run daily operations | [Operator guide](operator-guide/README.md) |
| Understand the task and approval model | [Operational workflow](workflow.md) |
| Use the CLI or TUI | [CLI and TUI reference](cli-reference.md) |
| Use the web console | [Web console guide](web-console.md) |
| Call the API or SDK from scripts | [Scripting and SDK](scripting-and-sdk.md) |
| Configure schedules and automation | [Automation guide](automation.md) |
| Connect AI agents and MCP clients | [AI tools and agents](ai-tools.md) |
| Operate and secure the deployment | [Security and operations](security-and-operations.md) |
| Diagnose problems | [Troubleshooting](troubleshooting.md) |
| Prepare a production release | [Production release checklist](production-release.md) |
| Back up and recover a self-contained instance | [Backup and recovery runbook](backup-recovery.md) |
| Record release-candidate evidence | [Release evidence template](release-evidence-template.md) |
| Deploy on one Linux host | [Systemd deployment](../deploy/README.md) |
| Build or extend the application | [Developer guide](developer-guide/README.md) |
| Understand current gaps | [Known limitations](KNOWN_LIMITATIONS.md) |

## Product map

VPS Tools has four layers.

1. **Control plane.** The API authenticates the actor, evaluates policy, validates targets and parameters, creates approvals or executions, and records audit events.
2. **Interfaces.** The CLI, TUI, web console, SDK, and MCP server all call the same API behaviour.
3. **Execution.** A runner claims leased targets, runs the approved work, submits output, and supports recovery after a runner interruption.
4. **Storage and scheduling.** The default tier uses SQLite, encrypted local artefacts, database polling, and an embedded scheduler. Larger backends are configuration extension points.

## User roles

- **Junior engineer.** Runs permitted runbooks, checks status, and escalates blocked work.
- **Senior engineer.** Authors, publishes, schedules, and reviews runbooks and can approve work where policy permits.
- **Owner or administrator.** Manages servers, runners, identity, configuration, and operational recovery.
- **Auditor.** Reviews executions, approvals, output references, and audit history without needing execution access.

## Documentation rules

Examples in these guides assume the self-contained deployment and the seeded development users. Replace those values for a real installation. A queued execution is not a successful execution. Always check the final status and audit trail.
