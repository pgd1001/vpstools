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

The web package includes a browser smoke test for the console shell, primary navigation, runbook search, guided task entry, preflight feedback, approval denial with a decision note, requester cancellation of queued work, and identity switching. Run `npm run smoke` from `apps/web` against a built Next.js server. CI installs Chromium and runs the check against both production-authentication and development-authentication builds.

For production web access, configure the OIDC variables described in [OIDC with ZITADEL](operator-guide/OIDC_ZITADEL.md). The browser receives an encrypted session cookie rather than a provider access token.

## Main views

- **Tasks and runbooks.** Search runbooks, inspect risk and permission, open the guided task form, run preflight, and submit work.
- **Schedules.** Create fixed-interval schedules as a senior engineer, inspect last errors, disable future runs, and pause or resume all new scheduled automation during an incident.
- **Approvals.** Open an approval brief to review the requester, action, risk, exact target snapshot, proposed parameters, rollback and verification plans, timestamps, and expiry. Approve from the brief or deny with a required decision note. The console keeps the brief open if the decision request fails so the operator can correct or retry it.
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

For approval work, open the request ID before deciding. The brief is the decision record, not just a queue row. Check the target snapshot and expiry, then record a useful explanation when denying a request. A failed decision is shown in the status area and does not discard the open brief or its note.

The console displays API errors in the status area and preserves entered form values when possible. If the page reports stale data, use Refresh before making a second request.

Pausing automation is organisation-wide. It holds new scheduler submissions but does not cancel work already queued, which is shown directly in the control panel.
