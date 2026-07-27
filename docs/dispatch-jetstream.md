# JetStream dispatch bridge

`JOB_DISPATCH=jetstream` is the first external dispatch increment. It is a notification bridge, not a replacement for the API queue.

The API still owns the execution target lease. After an execution transaction commits, the API publishes one metadata-only notification per pending target. The notification contains the target ID, execution ID, and attempt number. It never contains the command, SSH details, credentials, or lease ID.

The runner creates or reuses a durable pull consumer and uses explicit acknowledgements. It receives a notification, calls `GET /api/v1/jobs/next` with the target ID, and acknowledges the notification before executing the returned job. Duplicate notifications are safe because the API's conditional lease claim accepts only one active runner. A failed API claim is negatively acknowledged with a one-second delay. JetStream limits redelivery with `NATS_MAX_DELIVER`, which defaults to five and cannot exceed twenty.

The runner also performs the existing database claim poll when no notification is available. This is deliberate. A publish can fail after the database transaction commits, and a retry can become eligible after its original notification has already been acknowledged. The fallback keeps the database queue recoverable while the outbox pattern is still outside this increment.

## Configuration

The API and runner must use the same stream, subject, durable consumer, and NATS account.

```text
JOB_DISPATCH=jetstream
NATS_URL=nats://127.0.0.1:4222
NATS_STREAM=SVRTOOLS_JOBS
NATS_SUBJECT=svrtools.jobs.available
NATS_DURABLE=svrtools-runners
NATS_MAX_DELIVER=5
NATS_ACK_WAIT=30s
NATS_DUPLICATE_WINDOW=2m
```

Both services fail closed at startup when the selected dispatcher is unsupported, the NATS URL is missing, the stream settings are incomplete, or the JetStream stream and consumer have incompatible settings. The stream uses file storage, work-queue retention, a seven-day age limit, and server-side message ID de-duplication. The consumer is durable, pull-based, subject-filtered, explicit-ack, and bounded to one outstanding acknowledgement.

The API supports PostgreSQL metadata as an opt-in backend and applies its versioned migrations at startup. `EVENT_BUS=nats` and `SCHEDULER=external` remain unsupported combinations in this repository and are rejected at API startup. JetStream does not change that boundary.

## Exact limitations

- This is not a full JetStream execution queue. Commands and results still travel through the API, and the database lease remains authoritative.
- There is no transactional database outbox yet. A committed execution can therefore require the runner's database fallback if NATS publication fails.
- Notification publication currently covers raw executions, direct runbook executions, approved executions, and scheduled executions. Later retry eligibility is found by the fallback poll rather than by a new outbox record.
- A notification is acknowledged before remote execution. A runner crash after the API claim and before result submission is recovered by the existing lease expiry path. A remote side effect can still repeat after lease expiry, so commands must remain safe to retry.
- A shared durable consumer is intended for one runner fleet in one NATS account. Separate fleets need separate durable names and suitable API runner scopes.
- No live NATS server is needed for the unit tests. Stream creation, consumer creation, server-side redelivery, and network recovery require an integration environment with JetStream enabled.

## Verification

```powershell
go test ./packages/dispatch ./packages/config ./apps/runner
go test ./...
go vet ./...
```

The focused tests use fake publisher and consumer boundaries. They do not connect to NATS.
