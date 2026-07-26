# Web console guide

## Start the console

```bash
cd apps/web
npm install
npm run dev
```

Open `http://localhost:3000`. The console proxies API calls through its Next.js routes. Set `API_INTERNAL_URL` or `NEXT_PUBLIC_API_URL` when the API is not at `http://localhost:8080`.

## Development identity

For a local demo, set `NEXT_PUBLIC_DEV_AUTH=true`. The user switcher then selects the seeded identity in local storage. This is a development convenience, not an authentication system.

For production web access, configure the OIDC variables described in [OIDC with ZITADEL](operator-guide/OIDC_ZITADEL.md). The browser receives an encrypted session cookie rather than a provider access token.

## Main views

- **Tasks and runbooks.** Search runbooks, inspect risk and permission, open the guided task form, run preflight, and submit work.
- **Schedules.** Create fixed-interval schedules as a senior engineer, inspect last errors, and disable future runs.
- **Approvals.** Review pending requests and approve or deny with a recorded note.
- **Executions.** Monitor status, open execution detail, inspect timeline events, target status, and output.
- **Servers.** Browse inventory, check health, add servers, and archive servers where permitted.
- **Runners.** Inspect runner heartbeat state and revoke a runner where permitted.
- **Audit.** Search recent events by actor and review the action result.

## Recommended workflow

1. Open a runbook from the Tasks view.
2. Confirm the target and reason.
3. Select the declared parameters.
4. Use **Preview** to run preflight.
5. Read the risk and approval result.
6. Submit only when the intended action and target are clear.
7. Follow the execution detail view until a terminal state.

The console displays API errors in the status area and preserves entered form values when possible. If the page reports stale data, use Refresh before making a second request.
