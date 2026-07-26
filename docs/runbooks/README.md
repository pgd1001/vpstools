# Runbook Index

41 runbook templates across 7 categories. All live in `runbooks/*.yml` and are validated by `validate_runbooks_test.go`.

Runbooks are published to the API and searched via `vps runbook list` (CLI), TUI `screen 2` with `/` filter, or `?search=` on the API.

---

## Example (4)

Quick-start demos shipped with the seed data.

| Name | Risk | Tags | Description |
|---|---|---|---|
| `check-disk` | low | diagnostic, disk | Disk usage with `df -h` |
| `check-memory` | low | diagnostic, memory | Memory usage with `free -h` |
| `restart-nginx` | medium | service, nginx | Restart nginx service |
| `tail-logs` | low | logs, debugging | Tail application logs |

---

## Diagnostics (7)

Read-only system health and inventory. Safe to run on any server, any role.

| Name | Risk | Tags | Description |
|---|---|---|---|
| `system-info` | low | diagnostic, system, inventory | Kernel, CPU, RAM, filesystems, interfaces, uptime |
| `network-diag` | low | diagnostic, network, connectivity, dns | Ping, DNS resolution, HTTP check, traceroute, listening ports |
| `process-top` | low | diagnostic, process, performance | Top CPU/memory processes, zombie count, FD usage |
| `ssl-cert-check` | low | diagnostic, ssl, certificate, security | Cert expiry dates, days remaining, certbot status |
| `docker-stats` | low | diagnostic, docker, containers | Running containers, image count, disk usage, resource stats |
| `failed-auth-report` | low | diagnostic, security, authentication, ssh | Failed SSH logins, top source IPs, Fail2Ban status |
| `journal-check` | low | diagnostic, journal, logging, errors | Journal error count by service, kernel messages, critical events |

---

## Provisioning (7)

Server setup and baseline configuration. High/critical risk, requires senior approval for production.

| Name | Risk | Tags | Description |
|---|---|---|---|
| `base-hardened-ubuntu` | high | provisioning, security, ubuntu, hardening | UFW, SSH hardening, Fail2Ban, unattended upgrades, swap, sysctl |
| `docker-server` | high | provisioning, docker, portainer, containers | Docker CE + Compose + Portainer, firewall rules |
| `dokploy-install` | high | provisioning, dokploy, deployment, paas | Dokploy PaaS with Docker Compose on Ubuntu |
| `nextcloud-aio` | critical | provisioning, nextcloud, cloud, file-sync | Nextcloud All-in-One via Docker with backup mount |
| `seafile-install` | critical | provisioning, seafile, file-sync, cloud | SeaFile server with MariaDB + Redis via Docker |
| `hermes-agent` | medium | provisioning, monitoring, agent | Hermes Node Exporter + Loki agent with Docker |
| `ai-code-tools` | medium | provisioning, ai, development, coding | Claude Code CLI + OpenCode agent setup |

---

## AI Stack (6)

Self-hosted AI infrastructure. Medium-to-high risk, many pull external images and open ports.

| Name | Risk | Tags | Description |
|---|---|---|---|
| `ollama-openwebui-opencode` | medium | provisioning, ai, ollama, llm | Ollama + Open WebUI + OpenCode, GPU detection |
| `n8n-ai-starter-kit` | medium | provisioning, ai, n8n, automation | n8n + Ollama + Qdrant vector store + Flowise |
| `selfhosted-ai-package` | high | provisioning, ai, all-in-one | Supabase + n8n + Flowise + Ollama + Dify |
| `agixt-platform` | high | provisioning, ai, agents, automation | AGiXT agent platform with auto-update |
| `paperclip-ai` | medium | provisioning, ai, agents, orchestration | Paperclip agent orchestration, API-first |
| `ezlocal-ai` | medium | provisioning, ai, inference, llm | Local LLM inference (vLLM/Ollama), TTS, vision |

---

## Maintenance (6)

Routine upkeep. Medium risk, approval recommended for production.

| Name | Risk | Tags | Description |
|---|---|---|---|
| `system-update` | medium | maintenance, system, packages, update | `apt update && upgrade`, reboot-required check |
| `docker-cleanup` | medium | maintenance, docker, cleanup, prune | Remove stopped containers, dangling images, build cache |
| `log-cleanup` | medium | maintenance, logging, journald, logrotate | Rotate logs, journal vacuum, old gz cleanup |
| `config-backup` | low | maintenance, backup, configuration | Timestamped `/etc` tar.gz, S3 upload optional |
| `cert-renew` | medium | maintenance, ssl, certificate, certbot | Certbot renew + web service restart + health check |
| `service-rotate` | high | maintenance, service, restart, rolling | Rolling systemd restart with health gates per service |

---

## Security & Performance (7)

Audit and compliance. Low risk, read-only, safe for any environment.

| Name | Risk | Tags | Description |
|---|---|---|---|
| `audit-ports` | low | security, audit, ports, network | Listening ports vs allowlist, process mapping |
| `user-audit` | low | security, audit, users, ssh | Sudoers, UID 0 check, stale accounts, SSH keys |
| `fail2ban-status` | low | security, fail2ban, monitor, brute-force | Jail status, banned IPs, log tail |
| `ufw-status` | low | security, firewall, ufw, audit | Rule audit, permissive rule detection |
| `disk-usage-deep` | low | performance, disk, storage, analysis | du tree, inodes, large files, growth snapshot |
| `io-stat` | low | performance, disk, io, monitor | iostat, IOPS, latency, top I/O processes |
| `memory-report` | low | performance, memory, monitor, oom | Free, slab, NUMA, OOM scores, pressure |

---

## Recovery (4)

Emergency procedures. High risk, senior approval required for production.

| Name | Risk | Tags | Description |
|---|---|---|---|
| `service-restart` | high | recovery, service, restart, emergency | Force-restart systemd service with pre-state capture |
| `docker-restart` | high | recovery, docker, daemon, restart | Graceful dockerd restart with container drain |
| `swap-manage` | high | recovery, swap, memory, performance | Create, resize, or remove swap file |
| `emergency-cleanup` | high | recovery, disk, cleanup, emergency | Aggressive cleanup: apt cache, journal, Docker, old kernels |

---

## Usage

```bash
# List all runbooks (CLI)
vps runbook list

# List with search filter
vps runbook list | grep ssl

# Search via API
curl http://localhost:8080/api/v1/runbooks?search=docker

# Search in TUI (screen 2)
vps tui  # then press 2, then /

# View a specific runbook
vps runbook list --runbook ssl-cert-check

# Run a runbook
vps runbook run ssl-cert-check --target server:web01
```

## Risk summary

| Risk | Count | Requires approval? |
|---|---|---|
| critical | 2 | Yes (senior+ production) |
| high | 9 | Yes (senior+ production) |
| medium | 8 | Senior+ only, no approval |
| low | 22 | Any role |

**Total: 41 runbooks**
