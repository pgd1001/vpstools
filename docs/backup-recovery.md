# Backup and recovery runbook

The self-contained deployment backs up the SQLite database, the local encrypted artefact directory, and a checksum manifest. If the API generated `artifacts/.key`, the backup stores an authenticated encrypted envelope at `artifacts/.key.enc`, wrapped with the separately protected `BACKUP_ENCRYPTION_KEY`. The plaintext artefact key is never copied into a backup. If `ARTIFACT_ENCRYPTION_KEY` is configured, keep that original key in the external secret store and set `BACKUP_ENCRYPTION_KEY` only when a generated local key needs wrapping.

Verification checks the manifest structure, rejects duplicate or unlisted files, validates every recorded size and SHA-256 checksum, and runs SQLite `integrity_check` and `foreign_key_check` against the backed-up database. Restore runs the same SQLite checks on its temporary database before replacing any destination. A backup that opens successfully but contains SQLite integrity or referential errors is therefore rejected before it is published as recoverable.

## Create and verify a backup

```sh
./bin/backup -db /var/lib/vps-tools/svrtools.db \
  -artifacts /var/lib/vps-tools/data/artifacts \
  -output /var/lib/vps-tools/backups/20260727T020000Z
./bin/backup -mode verify -input /var/lib/vps-tools/backups/20260727T020000Z
```

The systemd timer runs the same sequence daily. It retains the configured local window and reports failures through systemd and the optional HTTPS webhook.

Set `BACKUP_ENCRYPTION_KEY` in the protected backup service environment before creating or restoring a backup that contains a generated local artefact key. It must be a separate base64-encoded 32-byte key. A backup with `artifacts/.key.enc` cannot be restored without it.

## Restore rehearsal

Restore into a disposable directory first. Never overwrite the live database during a rehearsal.

```sh
./bin/backup -mode restore \
  -input /var/lib/vps-tools/backups/20260727T020000Z \
  -db /tmp/vps-tools-rehearsal/svrtools.db \
  -artifacts /tmp/vps-tools-rehearsal/artifacts
./bin/backup -mode verify -input /var/lib/vps-tools/backups/20260727T020000Z
```

Start a temporary API against the restored paths, check `/api/v1/ready`, list executions, read a known artefact, and confirm audit history. Record the elapsed restore time and the backup timestamp in the release evidence. The repository test suite covers checksum failure, manifest validation, SQLite foreign-key validation, database restore, and artefact-key restoration, but a release still needs a host-level rehearsal.

## Recovery targets and off-host copies

The default timer provides a local recovery point. Its practical RPO is the interval between successful backups, normally 24 hours. Its RTO depends on the host, release transfer, restore size, and operator checks, so measure it during the rehearsal rather than assuming a value.

For production, set `BACKUP_REPLICATION_DIR` to a separate protected destination mounted under `/var/lib/vps-tools`, such as `/var/lib/vps-tools/replicated-backups`. The systemd backup job copies and verifies each backup there after the local manifest succeeds. Apply separate retention and immutability controls. An equivalent manual copy is:

```sh
rsync -a --delete /var/lib/vps-tools/backups/latest/ /srv/offhost/vps-tools/latest/
```

Do not use the same disk, credentials, or failure domain for the off-host copy. Keep at least one immutable or write-protected historical backup. Document the measured RPO and RTO in the release evidence before broad production rollout.

## Incident recovery order

1. Pause scheduled automation with `vps automation pause --reason "recovery"`.
2. Stop API and runner services if the live store may be changing.
3. Preserve the current database and artefact directory before replacing them.
4. Verify the selected backup manifest and checksums.
5. Provide `BACKUP_ENCRYPTION_KEY` when the manifest contains `artifacts/.key.enc`, then restore the database, artefacts, and encryption key together.
6. Start the API, check readiness, then start the runner.
7. Reconcile execution, artefact, and audit state before resuming automation.
8. Record the restore result, elapsed time, and any data loss in the incident record.
