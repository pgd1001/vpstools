# Automation guide

## Current automation model

The self-contained tier includes an embedded scheduler. Schedules are stored in SQLite and evaluated by the API process every ten seconds. A schedule uses a fixed interval, a published runbook, a target selector, a reason, and optional string parameters.

The scheduler:

1. Finds enabled schedules whose `next_run_at` is due.
2. Claims each due row by moving its next run time forward.
3. Resolves the target and checks environment and runbook policy.
4. Queues an execution as `user_automation`.
5. Records execution timeline and audit events.
6. Stores an error on the schedule if the run could not be queued.

At-least-once concerns still apply. Runbook actions should be idempotent, and operators should check execution state before manually retrying.

## Safety boundaries

- Only senior-authorized actors can create or disable schedules.
- The runbook must be published.
- Mixed environments are rejected.
- Production schedules need a reason.
- High and critical risk schedules are rejected from unattended execution.
- Every queued action has an automation actor and audit metadata.
- Disabling a schedule preserves its history.
- Senior operators can pause new scheduled submissions across the organisation during an incident. Existing queued executions are not cancelled by the pause.

## API example

Create a low-risk hourly check:

```bash
curl -X POST http://localhost:8080/api/v1/schedules \
  -H 'Content-Type: application/json' \
  -H 'X-VPS-User: user_senior' \
  -d '{
    "name": "hourly-uptime",
    "runbook_name": "check-uptime",
    "target": "server:srv_demo",
    "reason": "routine service health",
    "params": {},
    "interval_seconds": 3600
  }'
```

List and disable it:

```bash
curl -H 'X-VPS-User: user_senior' http://localhost:8080/api/v1/schedules
curl -X DELETE -H 'X-VPS-User: user_senior' http://localhost:8080/api/v1/schedules/<schedule-id>

# Emergency control
curl -X POST -H 'Content-Type: application/json' -H 'X-VPS-User: user_senior' \
  -d '{"reason":"incident response"}' http://localhost:8080/api/v1/automation/pause
curl -H 'X-VPS-User: user_senior' http://localhost:8080/api/v1/automation/status
curl -X POST -H 'X-VPS-User: user_senior' http://localhost:8080/api/v1/automation/resume
```

## Schedule design examples

Good candidates are read-only or naturally idempotent checks:

- Disk usage report
- Service status check
- Certificate expiry check
- Runner or agent heartbeat diagnostic
- Configuration drift report

Use a human approval workflow for changes such as package upgrades, service restarts, firewall changes, account changes, or production configuration edits. The current scheduler will block high and critical risk runbooks rather than convert them into approvals automatically.

## Future automation work

Event triggers, maintenance windows, health gates, post-execution verification, rollback, notifications, escalation rules, and richer retry policies remain planned. See [known limitations](KNOWN_LIMITATIONS.md).
