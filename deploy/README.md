# Single-host Linux service packaging

This directory contains the production-shaped packaging for a single Linux host running the API and one customer-managed runner. It assumes systemd, `curl`, a dedicated `vps-tools` service account, and a release bundle containing `api`, `runner`, `backup`, `artifact-migrate`, and `vps`.

The installer keeps immutable releases in `/opt/vps-tools/releases/<version>` and points `/opt/vps-tools/current` at the active release. Runtime state belongs to `/var/lib/vps-tools`. Configuration and secrets belong in these root-owned files, both mode `0600`.

```text
/opt/vps-tools/current/api
/opt/vps-tools/current/runner
/opt/vps-tools/current/backup
/opt/vps-tools/current/vps
/opt/vps-tools/releases/<version>/
/var/lib/vps-tools/svrtools.db
/var/lib/vps-tools/data/artifacts/
/var/lib/vps-tools/backups/
/etc/vps-tools/api.env
/etc/vps-tools/runner.env
/etc/vps-tools/known_hosts
```

## Install

Run from a checked-out release tree as root. The release directory must contain executable Linux binaries.

```sh
sudo ./scripts/install-systemd.sh ./release 0.4.0
sudoedit /etc/vps-tools/api.env
sudoedit /etc/vps-tools/runner.env
sudoedit /etc/vps-tools/backup.env
sudo chmod 600 /etc/vps-tools/*.env
sudo systemctl enable --now vps-tools-api.service vps-tools-runner.service
sudo systemctl enable --now vps-tools-healthcheck.timer
sudo systemctl enable --now vps-tools-backup.timer
sudo systemctl start vps-tools-backup-freshness.timer
sudo /usr/local/libexec/vps-tools/healthcheck.sh
```

The installer enables `vps-tools-backup-freshness.timer`. Start it after reviewing `backup.env`. It runs the freshness check 15 minutes after boot and every six hours afterwards. A failed check starts `vps-tools-backup-alert.service`, so failures are recorded in the journal and can be sent to the configured webhook.

The example API configuration uses the self-contained SQLite tier and stores the database and local artifacts under `/var/lib/vps-tools`. Set the real authentication settings before exposing the API beyond localhost. The installer creates an empty `/etc/vps-tools/known_hosts` file, which must be populated with trusted target keys before enabling real SSH execution. Set `VPS_RUNNER_TOKEN`, `SSH_KNOWN_HOSTS`, and the SSH settings before using the runner against real targets. `SIMULATE=true` is for local testing only.

The API service runs an HTTP health check after startup. The timer checks the API endpoint, the runner's loopback-only `/health` endpoint, and that both systemd services are active every minute. Prometheus can scrape `/metrics` from the API and the runner's loopback-only `/metrics`, preferably through a loopback-only or authenticated monitoring path. The runner health endpoint reports registration and completed-job counters, while its API heartbeat remains the control-plane liveness signal.

Prometheus examples and alert rules are in `deploy/monitoring/`. Copy them into the Prometheus configuration, keep the API and runner scrapes on protected loopback or authenticated paths, and route the resulting alerts to the organisation's normal incident system. The API metrics cover availability, readiness, request failures, queue depth, dead letters, active runners, enabled schedules, and rate-limit rejections. The runner metrics cover process availability, control-plane registration, and job counters. Backup failures and freshness failures remain visible through the systemd failure unit and optional webhook.

The backup timer runs daily at 02:15 UTC, verifies the manifest immediately after creating the backup, keeps the configured local retention window, and updates `backups/latest`. If the local artefact store generated `artifacts/.key`, set the separate protected `BACKUP_ENCRYPTION_KEY` in `/etc/vps-tools/backup.env`; the backup command stores only an authenticated encrypted key envelope. Set `BACKUP_REPLICATION_DIR` to a protected destination mounted under `/var/lib/vps-tools`, such as `/var/lib/vps-tools/replicated-backups`, to copy and verify each backup a second time. A verified run writes an atomic `backup-status.json`. The freshness timer checks the status age, the current `latest` manifest checksum, and the backup verifier result. It fails if the backup is missing, unverified, corrupted, or older than `BACKUP_MAX_AGE_SECONDS`, which defaults to 36 hours. The same checker can still be run manually with `BACKUP_STATUS_FILE=/var/lib/vps-tools/backups/backup-status.json scripts/check-backup-freshness.sh`. A failed backup or freshness check is recorded in the journal and can be sent to an HTTPS webhook with `VPS_BACKUP_ALERT_WEBHOOK`. The replicated destination still needs separate retention and immutability controls.

After configuring production identity, services, monitoring, and backups on the target host, run `/usr/local/libexec/vps-tools/production-acceptance.sh`. It checks production mode, authenticated `vps doctor` output, API and runner health, metrics endpoints, active systemd services, and backup freshness. Set `PRODUCTION_EVIDENCE_FILE=/var/lib/vps-tools/evidence/production-acceptance.md` to retain a redacted acceptance report with the release evidence. The command is a gate for the host checks, not a substitute for a restore rehearsal, alert-routing test, or measured RPO and RTO.

For a clean self-contained smoke test on Windows after building the four service binaries, run `powershell -ExecutionPolicy Bypass -File scripts/self-contained-smoke.ps1`. On Linux, run `sh scripts/self-contained-smoke.sh` against executable `api`, `runner`, `backup`, and `vps` binaries. Both scripts start the API and simulated runner in an isolated temporary directory, submit a CLI execution, verify a replicated backup, restore it into disposable paths, check readiness, and remove the temporary state. GoReleaser packages the five service and migration binaries for Windows, Linux, and macOS. Validate a Windows package with `powershell -ExecutionPolicy Bypass -File scripts/validate-release-layout.ps1`.

## Upgrade

Build or unpack the new release on the host, then run:

```sh
sudo ./scripts/upgrade-systemd.sh ./release 0.5.0
sudo systemctl status vps-tools-api.service vps-tools-runner.service
```

The script creates and verifies a pre-upgrade backup when the installed backup helper is available, stages the new binaries, stops the services, switches the `current` symlink atomically, starts the API, waits for `/api/v1/ready`, and starts the runner. If the API readiness check or either start fails, it restores the previous symlink and starts the previous release. Configuration and `/var/lib/vps-tools` are left in place. Schema compatibility still needs to be maintained for binary rollback.

## Rollback

List installed releases, select the previous version, and run:

```sh
sudo find /opt/vps-tools/releases -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | sort -V
sudo ./scripts/rollback-systemd.sh 0.4.0
sudo /usr/local/libexec/vps-tools/healthcheck.sh
```

Do not remove an old release until the newer release has passed its acceptance checks. Keep at least one known-good release for recovery. Database schema changes must remain compatible with the previous binary if a binary rollback is expected to work.

## Operations

```sh
sudo journalctl -u vps-tools-api.service -u vps-tools-runner.service -f
sudo systemctl status vps-tools-healthcheck.timer
sudo journalctl -u vps-tools-healthcheck.service --since today
sudo systemctl list-timers vps-tools-backup.timer
sudo systemctl list-timers vps-tools-backup-freshness.timer
sudo journalctl -u vps-tools-backup.service -u vps-tools-backup-freshness.service -u vps-tools-backup-alert.service --since today
sudo systemctl restart vps-tools-runner.service
```

The units restart failed processes after five seconds and stop retrying after five starts in one minute. Inspect `journalctl` before manually restarting a service that has hit the start limit. The service account has no login shell and the units use a read-only system filesystem with only `/var/lib/vps-tools` writable.
