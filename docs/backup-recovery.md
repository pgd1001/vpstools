# Backup and recovery runbook

The self-contained deployment backs up the SQLite database, the local encrypted artefact directory, and a checksum manifest. The artefact encryption key is either `artifacts/.key` or the externally supplied `ARTIFACT_ENCRYPTION_KEY`. Losing that key makes encrypted output unrecoverable, so it must be stored separately when an external key is configured.

## Create and verify a backup

```sh
./bin/backup -db /var/lib/vps-tools/svrtools.db \
  -artifacts /var/lib/vps-tools/data/artifacts \
  -output /var/lib/vps-tools/backups/20260727T020000Z
./bin/backup -mode verify -input /var/lib/vps-tools/backups/20260727T020000Z
```

The systemd timer runs the same sequence daily. It retains the configured local window and reports failures through systemd and the optional HTTPS webhook.

## Restore rehearsal

Restore into a disposable directory first. Never overwrite the live database during a rehearsal.

```sh
./bin/backup -mode restore \
  -input /var/lib/vps-tools/backups/20260727T020000Z \
  -db /tmp/vps-tools-rehearsal/svrtools.db \
  -artifacts /tmp/vps-tools-rehearsal/artifacts
./bin/backup -mode verify -input /var/lib/vps-tools/backups/20260727T020000Z
```

Start a temporary API against the restored paths, check `/api/v1/ready`, list executions, read a known artefact, and confirm audit history. Record the elapsed restore time and the backup timestamp in the release evidence. The repository test suite covers checksum failure, database restore, and artefact-key restoration, but a release still needs a host-level rehearsal.

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
5. Restore the database, artefacts, and encryption key together.
6. Start the API, check readiness, then start the runner.
7. Reconcile execution, artefact, and audit state before resuming automation.
8. Record the restore result, elapsed time, and any data loss in the incident record.
