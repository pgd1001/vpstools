# Getting started

## What you need

For the default tier, install Go 1.24 or newer and use a local machine or single server. Docker, PostgreSQL, S3, and NATS are not required. Docker remains useful for an SSH test target or later infrastructure experiments.

## Build the binaries

From the repository root:

```bash
go build -o bin/vps.exe ./apps/cli
go build -o bin/api.exe ./apps/api
go build -o bin/runner.exe ./apps/runner
```

On Linux or macOS, omit `.exe` from the output names.

## Start a local control plane

```powershell
.\bin\api.exe
```

The first start creates and migrates `svrtools.db`, seeds a demo organisation and users, and creates `data/artifacts`. The API starts with these defaults:

```text
DATABASE_DRIVER=sqlite
DATABASE_URL=./svrtools.db
ARTIFACT_STORE=local
ARTIFACTS_DIR=./data/artifacts
JOB_DISPATCH=database
SCHEDULER=embedded
EVENT_BUS=disabled
```

The local artefact store creates `data/artifacts/.key` with service-account permissions. Protect this key with the database backup. Without it, encrypted artefacts cannot be restored.

Verify the service:

```bash
curl http://localhost:8080/api/v1/health
```

The response reports the active deployment tier and backend choices.

## Select an identity in development

The seeded users are:

| User ID | Role | Intended use |
|---|---|---|
| `user_senior` | `senior_engineer` | Author and review runbooks |
| `user_junior` | `junior_engineer` | Run permitted tasks |
| `user_auditor` | `auditor` | Review evidence |

Set the CLI identity with `VPS_USER`:

```powershell
$env:VPS_USER = "user_senior"
.\bin\vps.exe whoami
```

The development API accepts the `X-VPS-User` header. Do not expose this development identity mechanism to an untrusted network. Use the documented OIDC setup for production web access.

## Start a runner

For a local smoke test, use simulation mode:

```powershell
$env:VPS_API_URL = "http://localhost:8080"
$env:SIMULATE = "true"
.\bin\runner.exe
```

For real SSH, configure the runner's API URL, runner credential, SSH target details, and trusted known-hosts file. Do not disable host verification to make a connection work.

## Run the first task

In another terminal:

```powershell
$env:VPS_USER = "user_junior"
.\bin\vps.exe runbook list
.\bin\vps.exe runbook run check-uptime --target server:srv_demo --wait
```

The runner claims the job, simulates or executes the command, and returns a terminal execution state. Inspect it later with:

```bash
vps exec list --mine --output json
vps audit search --limit 20
```

## Build and test the web console

```bash
cd apps/web
npm install
npm run dev
```

Open `http://localhost:3000`. In development, enable the local identity switcher with `NEXT_PUBLIC_DEV_AUTH=true`. Production web access should use the OIDC flow.

## Back up the first installation

```bash
make backup
```

The backup contains a SQLite snapshot, encrypted artefacts, and a manifest. Store the output directory somewhere separate from the live data directory and test a restore before relying on it.
