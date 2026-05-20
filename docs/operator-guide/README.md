# VPS Tools — Operator Guide

## Prerequisites

- Go 1.24+ (to build from source)
- Docker (optional, for SSH test target)
- Linux, macOS, or Windows

## Quick Start (Local Dev)

### 1. Build
```bash
go build -o bin/vps.exe ./apps/cli
go build -o bin/api.exe ./apps/api
go build -o bin/runner.exe ./apps/runner
```

### 2. Start the API
```bash
.\bin\api.exe
```
Uses embedded SQLite (`svrtools.db`). Auto-migrates and seeds on first run.

### 3. Verify connectivity
```bash
.\bin\vps.exe whoami
```
```
User:   senior@demo.local
Org:    Demo Org
Role:   senior_engineer
```

### 4. Add a server
```bash
.\bin\vps.exe server add web-01 --hostname web01.local --environment staging
```

### 5. Run a health check
```bash
.\bin\vps.exe server check srv_demo
```

### 6. Execute a command
```bash
.\bin\vps.exe exec server:demo -- uptime
```

### 7. Start the runner
```powershell
$env:SIMULATE = "true"
.\bin\runner.exe
```

### 8. Verify audit trail
```bash
.\bin\vps.exe audit search --limit 5
```

## Working with Different Users

Set `VPS_USER` variable to switch identity:
```bash
$env:VPS_USER = "user_junior"
.\bin\vps.exe whoami    # Shows junior_engineer
.\bin\vps.exe exec server:demo -- uptime  # DENIED - juniors cannot run raw commands
```

## Creating and Running Runbooks

### Create
```bash
.\bin\vps.exe runbook create check-disk --title "Check Disk Space" --command "df -h" --risk low
```

### Publish
```bash
.\bin\vps.exe runbook publish check-disk
```

### Run (as junior)
```bash
$env:VPS_USER = "user_junior"
.\bin\vps.exe runbook run check-disk --target server:demo
```

## Production Approval Workflow

1. Create a production server:
```bash
.\bin\vps.exe server add web-prod --hostname prod.local --environment production
```

2. Create a high-risk runbook:
```bash
.\bin\vps.exe runbook create restart-nginx --title "Nginx Restart" --command "systemctl restart nginx" --risk high --environment production
```

3. Publish:
```bash
.\bin\vps.exe runbook publish restart-nginx
```

4. Run (requires approval):
```bash
.\bin\vps.exe runbook run restart-nginx --target server:web-prod --reason "deploy v2.3"
# Output: Approval required — ID: apr_xxxxxxxxxx
```

5. Approve:
```bash
.\bin\vps.exe approvals list
.\bin\vps.exe approvals approve apr_xxxxxxxxxx
```

## TUI

```bash
.\bin\vps.exe tui
```
- `1` Servers, `2` Runbooks, `3` Executions, `4` Approvals, `5` Audit
- `q` Back/Quit, `h` Help

## Web Console

```bash
cd apps/web
npm install
npm run dev
```
Open http://localhost:3000

## CLI Output Formats

All list/detail commands support `--output json`:
```bash
.\bin\vps.exe server list --output json
.\bin\vps.exe audit search --limit 10 --output json
```

## Target Formats

- `server:<id|name>` — single server
- `tag:<key>=<value>` — all servers matching tag
- `<name>` — direct server name match

## Configuration

| Variable | Purpose | Default |
|---|---|---|
| `VPS_USER` | CLI user identity | `user_senior` |
| `VPS_API_URL` | API address | `http://localhost:8080` |
| `API_PORT` | API listen port | `8080` |
| `DB_PATH` | SQLite database path | `svrtools.db` |
| `SIMULATE` | Runner simulate mode | omit for real SSH |
