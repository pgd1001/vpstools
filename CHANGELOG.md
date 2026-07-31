# Changelog

All notable changes to VPS Tools are recorded here. The project follows Semantic Versioning. Until the first stable release, the `0.x` series may still change behaviour where the current documentation and known limitations call it out.

## [Unreleased]

Changes after `0.1.0-beta.1` will be recorded here first. Once a change is ready for a release, move it into a dated version section and create the matching Git tag.

### Changed

- **Breaking: SSH credentials are now per server and held only by the runner.** A single fleet-wide `SSH_PASSWORD` previously authenticated every target and one shared `SSH_KNOWN_HOSTS` file governed host trust, so one compromised credential exposed the whole inventory and no host could be rotated independently. Each server now records an SSH credential reference and the SHA256 fingerprint of its host key. The runner resolves the reference against its own `SSH_CREDENTIALS_DIR` and pins the host key on every connection, refusing to connect to a server missing either value rather than connecting unverified. Private key material never reaches the control plane, so an API database compromise no longer yields fleet access. Existing deployments must create the credential directory, place one file per credential in it, and record a fingerprint for every server with `ssh-keyscan -p <port> <host> | ssh-keygen -lf -` before real execution resumes. `SSH_PASSWORD` and `SSH_KNOWN_HOSTS` are no longer read.
- **Breaking: `JOB_SIGNING_KEY` is now required.** The API signs every dispatched job over the command, host, port, user, timeout, credential reference, host key fingerprint, and lease, and the runner refuses any job it cannot verify. The API and every runner must share the same key of at least 32 characters. Existing deployments must set it before upgrading; both processes exit at startup without it.
- **Breaking: the runner job endpoints require a runner-bound credential.** Registration now exchanges the bootstrap credential for one bound to the runner identity, and claim, renew, result, and heartbeat accept only the bound form. An organisation-wide credential could previously claim work scoped to any runner in the organisation, which made runner scopes unenforceable.
- **Breaking: the API no longer infers an identity.** A request with no credential is unauthenticated rather than resolved to a senior engineer, and a runner request with no credential no longer resolves to the demo organisation. Development header identity additionally requires an explicitly non-production environment; any unrecognised or unset environment name is now treated as production.

### Fixed

- Audit search silently dropped every event with no actor, which is all system-generated ones: runner registration, credential issuance, execution completion, and scheduled runs. Nullable columns are now coalesced, and an undecodable row returns an error rather than being skipped.
- The web console proxy sent no identity in development mode, so every proxied request failed once the API stopped inferring one. It now names the actor explicitly and strips identity headers supplied by the caller.
- SQLite read traffic queued behind the single writer, and lease reconciliation ran a full scan of an organisation's running targets on every job claim, so a handful of polling runners could saturate the API. Reads now use a separate WAL pool and reconciliation is throttled per organisation, with the embedded scheduler sweeping so abandoned work is still dead-lettered.

### Added

- `vps server add --ssh-credential-ref` and `--ssh-host-key-fingerprint`, with the same fields on the server API, Go SDK, web console, and server detail responses. The CLI warns at registration when either is missing, and the web console server list marks any server that is not yet executable. A server update that omits these fields preserves the stored values, so a client that does not manage SSH identity cannot silently clear a host key pin.
- `packages/sshcreds`, the runner-local keystore that resolves a credential reference to key material. References are validated before use, so a reference from the API cannot escape the keystore directory.
- `svrtools_runner_jobs_rejected_total`, which counts jobs a runner refused because the signature did not verify. It is non-zero only when something is dispatching jobs the runner does not trust.
- Schema parity is now derived from both dialects and compared in full, so a column added to SQLite or PostgreSQL and forgotten in the other fails the build.

## [0.1.0-beta.1] - 2026-07-28

This is the first named beta baseline for the implemented MVP. It consolidates the work completed from the Phase 0 architecture spike through the Phase 7 hardening work and the subsequent operations, backend, recovery, and AI additions.

### Added

- CLI, API, runner, audit, inventory, runner registration, heartbeats, target resolution, execution status, cancellation, retries, leases, and result receipts.
- Role-based access for owners, administrators, senior engineers, junior engineers, and auditors, with deny-by-default policy checks, approval workflows, tenant isolation, and audit hash-chain verification.
- Immutable, versioned YAML runbooks with parameter validation, target constraints, risk classification, production reasons, publishing, search, and a catalog of 41 templates.
- Bubble Tea TUI support for servers, runbooks, executions, approvals, schedules, and audit, including runbook search and guided operational flows.
- Next.js web console with CRUD operations, control-plane readiness, development identity support, and ZITADEL/OIDC login, callback, session, and logout routes.
- Embedded interval schedules with explicit automation identity, organisation-wide pause and resume controls, conservative risk rules, and audit records.
- Expiring bearer API tokens for CLI, SDK, and automation clients, plus production preflight checks through `vps doctor` and the MCP doctor tool.
- Local encrypted artefact storage, atomic writes, large-output references, backup manifests, verification, restore, key protection, and systemd backup freshness checks.
- PostgreSQL metadata runtime with versioned migrations, dialect-aware handlers, schema verification, optional tenant RLS, and tenant-bound runner requests.
- S3-compatible artefact storage with client-side encryption, signed read URLs, verified migration manifests, safe identifiers, and combined PostgreSQL plus S3 recovery helpers.
- Database-authoritative JetStream notification dispatch, with validation and duplicate-safe claiming while the database remains the source of truth.
- Bounded, read-only AI analysis through an OpenAI-compatible provider boundary, redaction contracts, SDK and CLI access, web execution analysis, and a local stdio MCP server.
- GoReleaser packaging, checksums, SBOM generation, release evidence validation, packaged self-contained smoke tests, Windows archive validation, deployment manifest checks, production acceptance checks, and repository security governance.

### Changed

- The default deployment is now clearly documented as a self-contained SQLite, local encrypted artefact, database queue, and embedded scheduler tier.
- Extended PostgreSQL, S3, and JetStream settings fail closed when incomplete instead of silently falling back to local services.
- Production guidance now separates implemented extensions from future enterprise milestones such as external scheduling, NATS event publishing, and an independent horizontally scaled queue.
- Recovery documentation now covers local restore, PostgreSQL dump validation, S3 manifest verification, combined extended recovery, and safe manifest path handling.

### Security

- Added secret redaction at runner and API boundaries, encrypted large output, protected generated backup keys, rate limits, security headers, origin checks, runner scope checks, and cross-organisation isolation tests.
- Added PostgreSQL RLS bootstrap checks and fail-closed startup behaviour when the configured tenant isolation policy is not ready.

### Verification

- The repository CI covers Go tests and race tests, vet, vulnerability scanning, PostgreSQL integration, runbook validation, self-contained Linux and Windows smoke tests, MCP checks, web builds and browser smoke tests, package layout, deployment manifests, and release evidence.

## Earlier implementation history

The following milestones are retained as the project history behind the beta baseline.

### 2026-05-18 to 2026-05-20, Phases 0 to 7

- Phase 0 proved the CLI to API to runner to SSH to audit path.
- Phase 1 added the monorepo foundations, CI, protobuf generation, sqlc, Goose, structured logging, Viper configuration, and initial documentation.
- Phase 2 added server inventory, tags, runner registration, and heartbeats.
- Phase 3 added target resolution, execution lifecycle state, cancellation, and audit events.
- Phase 4 added role-based access, policy evaluation, and deny-by-default execution.
- Phase 5 added versioned runbooks, delegated execution, approvals, target snapshots, and approval audit history.
- Phase 6 added the Bubble Tea TUI and the Next.js web console.
- Phase 7 added output redaction, security tests, tenant and runner scope checks, known limitations, operator guidance, and GoReleaser packaging.
- Follow-up fixes covered web CORS, TUI keyboard navigation, and removal of accidentally committed SQLite WAL files.

### 2026-07-08, operational catalogue

- Added 24 maintenance, diagnostics, security, and recovery runbook templates, bringing the catalogue to 41.
- Added TUI runbook search and the API `?search=` filter.

### 2026-07-25 to 2026-07-27, production and extension work

- Added web CRUD and ZITADEL/OIDC authentication, self-contained deployment backends, expanded operator documentation, AI tooling, the production doctor, MCP doctor, readiness reporting, production acceptance checks, and release safeguards.
- Added the database composition and SQL dialect foundations needed for PostgreSQL runtime support.
- Added read-only AI analysis, S3 storage and migration tooling, signed S3 download URLs, verified manifests, safe artefact identifiers, and local restore from verified S3 objects.
- Added PostgreSQL startup, migration portability, tenant RLS, non-superuser RLS integration coverage, tenant-bound runner requests, and PostgreSQL backup and restore helpers.
- Added combined extended-tier recovery helpers and hardened their manifest path validation.
- Corrected release gates, Windows package selection, Windows release validation, CI conditions, SBOM snapshot handling, and Bash execution for extended recovery helpers.

## Versioning notes

- `VERSION` is the source-controlled product version for local builds and documentation.
- Release builds receive the exact tag version through GoReleaser linker flags.
- The CLI exposes the value through `vps version`, and the API exposes it through `/api/v1/health`.
- Runbook `metadata.version` is a separate per-runbook revision and must not be confused with the product release version.

[Unreleased]: https://github.com/pgd1001/svrtools/compare/v0.1.0-beta.1...HEAD
[0.1.0-beta.1]: https://github.com/pgd1001/svrtools/releases/tag/v0.1.0-beta.1
