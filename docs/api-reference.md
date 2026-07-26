# API reference

The API listens on `http://localhost:8080` by default. JSON is used for request and response bodies.

## Authentication

Local development requests use:

```http
X-VPS-User: user_senior
```

The API resolves that user inside the configured organisation. The web OIDC path uses the internal identity headers behind the configured shared secret. Runner job endpoints use a runner credential in `X-VPS-Runner-Token`.

Do not put development identity headers on a public listener. Use OIDC provisioning and a protected reverse proxy for production access.

## Response conventions

Successful responses return a JSON object. Errors normally include:

```json
{
  "error": "human-readable message",
  "reason": "machine-readable reason",
  "next": "permitted next action"
}
```

Sensitive commands are returned as redacted previews. The raw execution command is retained only for the runner execution boundary.

## Public endpoints

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/health` | Health and active deployment tier |
| GET | `/api/v1/whoami` | Current actor and role |
| GET, POST | `/api/v1/servers` | List or create inventory records |
| GET, PATCH, PUT, DELETE | `/api/v1/servers/:id` | Inspect, update, or archive a server |
| POST | `/api/v1/servers/:id/check` | Run a server health check |
| GET | `/api/v1/runners` | List runners |
| POST | `/api/v1/runners/manage` | Register a runner |
| PATCH, DELETE | `/api/v1/runners/:id` | Update or revoke a runner |
| POST | `/api/v1/runners/heartbeat` | Runner heartbeat |
| POST | `/api/v1/runners/registration-token` | Create a runner registration credential |
| GET, POST | `/api/v1/executions` | List or create direct executions |
| GET | `/api/v1/executions/:id` | Read execution detail and targets |
| POST | `/api/v1/executions/:id/cancel` | Cancel a pending or queued execution |
| GET, POST | `/api/v1/runbooks` | List or create runbooks |
| GET, PUT, PATCH, DELETE | `/api/v1/runbooks/:name` | Read, change, or archive a runbook |
| POST | `/api/v1/runbooks/:name/publish` | Publish the current runbook version |
| POST | `/api/v1/runbooks/:name/run` | Preflight or request runbook execution |
| GET | `/api/v1/approvals` | List approval requests |
| GET | `/api/v1/approvals/:id` | Read an approval brief |
| POST | `/api/v1/approvals/:id/approve` | Approve a request |
| POST | `/api/v1/approvals/:id/deny` | Deny a request with a note |
| GET, POST | `/api/v1/schedules` | List or create interval schedules |
| DELETE | `/api/v1/schedules/:id` | Disable a schedule |
| GET | `/api/v1/audit` | Search audit events |

Runner-only endpoints are `GET /api/v1/jobs/next` and `POST /api/v1/jobs/result`. They require a valid runner credential and must not be exposed directly to end users.

## Runbook preflight

```http
POST /api/v1/runbooks/check-uptime/run
Content-Type: application/json
X-VPS-User: user_junior

{
  "target": "server:srv_demo",
  "reason": "routine check",
  "params": {},
  "dry_run": true
}
```

Preflight returns `status: preflight`, target count, environment, risk, approval requirement, target snapshot, and a redacted command preview. It must not create an execution.

## Requesting a runbook

Remove `dry_run` after the user or an approved automation policy allows submission. A low-risk request returns an execution ID. A high-risk request returns an approval ID and `awaiting_approval` status.

## Listing executions

`GET /api/v1/executions` accepts `status`, `limit`, `mine=true`, and `delegated=true` filters. `GET /api/v1/executions/:id` includes timeline events and per-target output fields.

## Pagination and limits

The current endpoints use bounded `limit` parameters rather than cursor pagination. Clients should request a practical limit, handle an empty list, and not assume the first page contains all historical evidence.
