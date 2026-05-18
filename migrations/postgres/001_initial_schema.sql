-- +goose Up
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE organisations (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
    email       TEXT NOT NULL,
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    user_id         TEXT NOT NULL REFERENCES users(id),
    role            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE servers (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    name            TEXT NOT NULL,
    hostname        TEXT NOT NULL,
    environment     TEXT NOT NULL DEFAULT 'staging',
    tags            JSONB NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'unknown',
    last_seen_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE executions (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    actor_id        TEXT NOT NULL REFERENCES users(id),
    command         TEXT NOT NULL,
    command_hash    TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'queued',
    reason          TEXT,
    dry_run         BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ
);

CREATE TABLE execution_targets (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id    TEXT NOT NULL REFERENCES executions(id),
    server_id       TEXT NOT NULL REFERENCES servers(id),
    status          TEXT NOT NULL DEFAULT 'pending',
    exit_code       INTEGER,
    stdout          TEXT,
    stderr          TEXT,
    error           TEXT,
    duration_ms     BIGINT,
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ
);

CREATE TABLE audit_events (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    actor_id        TEXT,
    action          TEXT NOT NULL,
    target_type     TEXT NOT NULL,
    target_id       TEXT,
    result          TEXT NOT NULL,
    metadata_json   JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_events_org_time ON audit_events(organisation_id, created_at);
CREATE INDEX idx_audit_events_actor ON audit_events(actor_id);
CREATE INDEX idx_audit_events_action ON audit_events(action);
CREATE INDEX idx_executions_org_status ON executions(organisation_id, status);
CREATE INDEX idx_servers_org ON servers(organisation_id);
CREATE INDEX idx_execution_targets_exec ON execution_targets(execution_id);

-- +goose Down
DROP TABLE IF EXISTS execution_targets CASCADE;
DROP TABLE IF EXISTS executions CASCADE;
DROP TABLE IF EXISTS servers CASCADE;
DROP TABLE IF EXISTS memberships CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS organisations CASCADE;
DROP TABLE IF EXISTS audit_events CASCADE;
