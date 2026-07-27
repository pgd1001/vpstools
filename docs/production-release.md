# Production release checklist

This checklist defines the minimum bar for a supported self-contained production release. PostgreSQL, S3, and JetStream remain a separate enterprise milestone until their adapters and migration tests are implemented. The repository does not assume a software licence. Choose and add the appropriate licence before a public release.

## Release gates

### Identity and security

- [x] `VPS_ENV=production` is set, development header authentication is disabled, and the process refuses unsafe production settings.
- [x] CLI and SDK users authenticate with expiring bearer tokens or short-lived service credentials.
- [x] Web sessions use OIDC, secure cookies, a managed session secret, and origin-checked state-changing requests.
- [x] Runner registration credentials are one-hour, runner-bound credentials. Rotation revokes the previous credential, and runner revocation invalidates active credentials.
- [x] TLS is terminated at a documented reverse proxy or supported service boundary.
- [x] Rate limits and security headers are enabled.
- [x] Tenant isolation, role checks, runner scope checks, and secret redaction pass automated tests.

### Data and recovery

- [x] SQLite uses WAL mode, foreign keys, a bounded connection pool, and tested lease recovery.
- [x] Local artefacts are encrypted with `ARTIFACT_ENCRYPTION_KEY` or a protected generated key.
- [x] Backups include the database, encrypted artefacts, key-recovery guidance, checksum manifest, and artifact inventory.
- [ ] Backup integrity can be checked and a complete restore has been rehearsed. The isolated smoke tests exercise local backup, replication, verification, restore, restored API startup, encrypted artefact readiness, identity, execution history, and audit history. A release still needs the same sequence against the production host and its real encryption-key recovery path.
- [ ] Retention, off-host replication, RPO, and RTO are documented. The systemd deployment supports a separately mounted replication destination, but each production installation still needs measured RPO/RTO and an exercised restore from the replicated copy.
- [x] A failed backup produces an operator-visible alert through systemd journal failure handling, with an optional HTTPS webhook.
- [x] Audit events are chained per organisation with SHA-256 event hashes, legacy local records are backfilled at startup, and auditors or senior operators can verify the chain.

### Execution safety

- [x] Published runbook versions are immutable and executions retain the exact version and target snapshot.
- [x] Parameter validation, shell-safe rendering, target restrictions, environment separation, approval rules, and production reasons are tested.
- [ ] Queue leases recover after runner failure without duplicate execution. Durable result receipts make accepted result replays idempotent, runners retry transient submission failures, and raw and runbook submissions support persisted actor-scoped idempotency keys. Exactly-once remote side effects after runner power loss remain open until command-level idempotency or a durable runner outbox is implemented and tested.
- [x] Cancel, retry, partial success, dead-letter, and reconciliation behaviour are defined and tested. Retry is bounded by `max_attempts`, uses short database-backed backoff, and exhausted work is dead-lettered.
- [x] High-risk work cannot run unattended without an approval-backed path.
- [x] Senior operators can pause or resume organisation-wide scheduled automation. Pausing stops new scheduler submissions and is audited.

### Operations and release engineering

- [x] CI uses the Go version in `go.mod` and runs Go tests, race tests, vet, vulnerability checks, web build, MCP checks, and runbook validation.
- [x] Reproducible binaries are built for supported operating systems and architectures.
- [x] Release artefacts include checksums, an SBOM, version metadata, and upgrade notes.
- [ ] A protected tag-based release publishes the verified artefacts and records provenance or signatures. The workflow builds a draft, validates the exact output, runs packaged smoke and release-evidence checks, creates and verifies a GitHub build-provenance attestation for `dist/checksums.txt`, then publishes the draft. The protected workflow still needs to run successfully and any additional signing policy still needs to be approved.
- [x] API, runner, and web services have documented startup, shutdown, upgrade, and rollback procedures.
- [ ] Metrics, structured logs, health checks, and alerts cover API, runner, queue, scheduler, storage, and backups. The API exposes bounded queue, dead-letter, active-runner, enabled-schedule, artefact-aware readiness, and local artefact-filesystem capacity signals. The runner exposes loopback-only health and metrics endpoints, and the systemd health check verifies them. The systemd backup job writes freshness evidence, verifies the current manifest and backup contents, and has a scheduled freshness failure unit. External collection, alert routing, and incident testing still need to be configured and exercised for each deployment.
- [ ] A clean-machine installation succeeds without Docker, PostgreSQL, S3, or NATS. CI now runs source-built Windows smoke, extracted Linux package smoke, and extracted Windows package smoke. A release candidate still needs the same check on the target host, including systemd installation and rollback.

The CI snapshot release check treats `dist/checksums.txt` and at least one CycloneDX or SPDX SBOM as required evidence. The protected tag workflow runs the repository test gates, packaged smoke test, archive-layout validator, and release-evidence validator before publishing a draft, then validates the final published output again. CI pins GoReleaser `v2.17.0`, Syft `v1.44.0`, and govulncheck `v1.6.0` under Go `1.26.3` for repeatable release tooling. Run `make release-check` locally when GoReleaser, Syft, `jq`, and a checksum tool are available. On Windows, Make uses `scripts/validate-release.ps1` automatically.

### User workflow

- [x] Junior engineers can find and complete permitted tasks without writing shell syntax through the guided web and TUI runbook flows, with preflight and role-aware permissions.
- [x] Approvers can see the complete request, risk, target snapshot, parameters, and declared rollback, verification, and evidence plan in the web console, TUI, API, and MCP approval brief.
- [ ] CLI, TUI, web console, API, SDK, and MCP expose consistent state and permission behaviour. The repository smoke path now exercises CLI, API, bearer-token identity, and live MCP read access together; the release candidate still needs the full parity audit.
- [x] Critical web and TUI workflows have automated coverage.
- [x] The documented self-contained path is exercised against built release binaries, including first-run health and identity checks, runbook execution, automation pause/resume and schedule create/list/disable, MCP access, backup verification and replication, restore, post-restore history checks, and audit-chain verification.

The current automated coverage includes TUI guided-runbook, approval-denial, queued-cancellation, schedule-creation, and schedule-disable views, plus a browser smoke covering production web-console boot, navigation, runbook search, guided task entry and preflight feedback, approval denial with a decision note, requester cancellation of queued execution, the OIDC sign-in entry point, and development-user switching. The browser smoke passes against both production and development-authenticated builds. The Windows and Linux self-contained smoke scripts exercise the documented release-binary workflow, including automation and recovery. Release-candidate validation against the real identity provider and production infrastructure is still required.

## Evidence required for sign-off

Keep the following with every release candidate:

Use the [release evidence template](release-evidence-template.md) so operational checks are recorded consistently.

- CI run URL and test results
- Version and build checksums
- SBOM documents and the output of the release evidence validator
- Clean-machine installation transcript
- Authentication and authorisation test report
- Backup and restore report with measured RPO and RTO
- Runner failure and lease-recovery test report
- Representative approval, execution, failure, retry, and audit records
- Security review findings and accepted residual risks
- Release notes and rollback instructions

## Release decision

The self-contained tier may be released for a controlled production pilot when every identity, recovery, execution-safety, and release-engineering gate is complete. A broader production release should wait until the user workflow and observability gates are also complete.
