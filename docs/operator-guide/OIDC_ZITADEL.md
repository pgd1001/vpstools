# ZITADEL OIDC setup

The web console uses the OIDC Authorization Code flow with PKCE. The browser never receives an access token. Next.js exchanges the code, verifies the ID token against ZITADEL's JWKS, and stores an encrypted, HttpOnly session cookie.

## ZITADEL application

Create a Web application in ZITADEL with:

- Redirect URI: `https://console.example.com/api/auth/callback`
- Post logout redirect URI: `https://console.example.com/`
- Authorization Code with PKCE
- Scopes: `openid profile email`

For local development, use `http://localhost:3000` equivalents.

## Environment

Copy `apps/web/.env.example` to the deployment environment. Set the same `VPS_WEB_SHARED_SECRET` on the web process and the API process. Set `SESSION_SECRET` to a random value of at least 32 characters and keep it private.

The API maps a verified ZITADEL subject to a pre-provisioned local user. The first login can bind by exact email. After that, the stored external subject is the stable mapping. The user's organisation membership and role remain controlled by VPS Tools, not by browser-supplied claims. ZITADEL project roles are not used as application roles.

If a user is not provisioned in the `users` and `memberships` tables, the API returns `OIDC user is not provisioned`.

For the current two operators, provision the local accounts with the roles you want before their first login. VPS Tools accepts `owner`, `admin`, `senior_engineer`, `junior_engineer`, and `auditor`. For example, the support account can use `junior_engineer` if it should have normal operator access. There is no `user` role in the VPS Tools policy model.

From the repository root, the included helper can do this for the local SQLite database:

```powershell
go run ./apps/api/cmd/provision-oidc --db svrtools.db --org org_demo --user-id user_paul --email paul@deegan.ie --display-name "Paul Deegan" --role admin
go run ./apps/api/cmd/provision-oidc --db svrtools.db --org org_demo --user-id user_support --email support@nualogic.com --display-name "Nualogic Support" --role junior_engineer
```

The command is safe to rerun. It updates an existing email or membership rather than creating a duplicate. Leave the external subject empty. The first verified ZITADEL login fills that binding.

## Production notes

- Use HTTPS for both console and API traffic.
- Do not set `NEXT_PUBLIC_API_URL` to a publicly reachable API in OIDC mode. Let the server-side proxy use `API_INTERNAL_URL`.
- Rotate `SESSION_SECRET` and `VPS_WEB_SHARED_SECRET` through the deployment secret manager.
- Use different random values for `SESSION_SECRET` and `VPS_WEB_SHARED_SECRET`. Never commit either value to `.env.example` or source control.
- Set `VPS_DEV_AUTH=false` or leave it unset.
- Configure MFA and passkeys in ZITADEL, with MFA required for privileged users.
- The logout route always clears the local session. Set `ZITADEL_PROVIDER_LOGOUT=true` only after registering the exact `ZITADEL_POST_LOGOUT_REDIRECT_URI` in ZITADEL. Otherwise the browser stays signed in at ZITADEL and a later login can complete silently, which is normal SSO behaviour.
