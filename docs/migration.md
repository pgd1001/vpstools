# Moving from self-contained to extended deployment

The self-contained tier is the default because it is easy to install, back up, and understand. The application reserves a path for larger teams to move to PostgreSQL, S3-compatible storage, and JetStream without changing the CLI, web console, runbooks, or AI tools. S3 artefact storage is now an explicit runtime option. PostgreSQL, JetStream, and the migration commands are not shipped as supported runtime features in the current release.

## Target configuration

Self-contained operation uses:

```text
DATABASE_DRIVER=sqlite
DATABASE_URL=./svrtools.db
ARTIFACT_STORE=local
ARTIFACTS_DIR=./data/artifacts
JOB_DISPATCH=database
SCHEDULER=embedded
EVENT_BUS=disabled
```

An extended deployment selects the external services explicitly:

```text
DATABASE_DRIVER=postgres
ARTIFACT_STORE=s3
JOB_DISPATCH=jetstream
SCHEDULER=external
EVENT_BUS=nats
```

External settings must be complete. VPS Tools should refuse a partially configured extended tier rather than falling back to local stores without an operator knowing.

## Expand and contract sequence

1. Take and verify a backup of SQLite metadata and local artefacts.
2. Export or replicate metadata into PostgreSQL while preserving IDs, timestamps, statuses, approvals, and audit history.
3. Copy local artefacts to S3-compatible storage, retaining stable artefact IDs and verifying checksums. The supported helper is `go run ./apps/api/cmd/artifact-migrate`.
4. Start reconciliation in read-only comparison mode.
5. Enable JetStream dispatch behind a feature flag and verify durable consumers, acknowledgements, redelivery limits, and idempotency.
6. Run new work through the extended backends while the previous stores remain available as a read-only fallback.
7. Compare executions, results, artefacts, and audit events across both stores.
8. Validate backup, replay, recovery, and duplicate-delivery behaviour.
9. Remove the fallback only after the comparison window has completed successfully.

## What must remain stable

Migration must preserve runbook IDs and immutable versions, server IDs, execution and job IDs, approval history, audit event IDs, artefact references, content hashes, timestamps, and terminal states. Stable IDs allow the web console, CLI, SDK, and AI tools to continue working while storage moves underneath them.

## Rollback planning

Keep the original SQLite database, artefact directory, encryption key, and backup manifest untouched until recovery validation is complete. If the extended tier produces a mismatch, stop new work, preserve both sets of records, and switch reads back to the previous stores while the discrepancy is investigated.

Do not delete the local source stores as part of the first migration. Storage cleanup is a separate change after restore testing and an agreed retention period.

## Current implementation boundary

The self-contained SQLite, local artefact, database polling, and embedded scheduler path is the supported default. PostgreSQL, S3, and JetStream are configuration targets and architectural extension points. The artifact helper is intentionally limited to local-to-S3 transfer and read-back verification. It doesn't migrate database metadata, delete local files, reconcile objects missing from the local source, or perform cutover.

## Local-to-S3 artifact migration

Set the S3 settings described in the deployment configuration, then run the helper while the local source remains available:

```text
go run ./apps/api/cmd/artifact-migrate
```

The helper decrypts each local `.bin` artifact through the local store, uploads the plaintext to S3 where the S3 store applies its configured encryption, and reads the object back. Objects with the same SHA-256 and size are skipped, so a rerun is safe. A different remote checksum stops the migration. Use `-force` only after investigating the conflict. The flag replaces the conflicting object and still requires successful read-back verification.

The command reports copied objects, skipped objects, and source bytes scanned. It never removes local artifacts or remote objects that aren't present locally. Keep the local directory, its encryption key, and a verified backup until the extended deployment has passed recovery testing.

