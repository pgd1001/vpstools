-- +goose Up
-- Reconcile databases created by the original PostgreSQL migrations with the
-- current self-contained SQLite runtime schema. Every statement is safe to
-- run against an already-upgraded database.

ALTER TABLE runner_credentials
    ADD COLUMN IF NOT EXISTS runner_id TEXT REFERENCES runners(id) ON DELETE CASCADE;

ALTER TABLE executions
    ADD COLUMN IF NOT EXISTS command TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS api_tokens (
    id              TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    user_id         TEXT NOT NULL REFERENCES users(id),
    name            TEXT NOT NULL,
    token_prefix    TEXT NOT NULL,
    token_hash      TEXT NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    last_used_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS automation_schedules (
    id                  TEXT PRIMARY KEY,
    organisation_id     TEXT NOT NULL REFERENCES organisations(id),
    created_by_user_id  TEXT NOT NULL REFERENCES users(id),
    name                TEXT NOT NULL,
    runbook_name        TEXT NOT NULL,
    target              TEXT NOT NULL,
    reason              TEXT NOT NULL,
    params              TEXT NOT NULL DEFAULT '{}',
    interval_seconds    INTEGER NOT NULL,
    next_run_at         TIMESTAMPTZ NOT NULL,
    enabled             INTEGER NOT NULL DEFAULT 1,
    last_run_at         TIMESTAMPTZ,
    last_error          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT automation_schedules_unique_name UNIQUE (organisation_id, name)
);

CREATE TABLE IF NOT EXISTS automation_controls (
    organisation_id     TEXT PRIMARY KEY REFERENCES organisations(id) ON DELETE CASCADE,
    paused              INTEGER NOT NULL DEFAULT 0,
    paused_at           TIMESTAMPTZ,
    paused_by_user_id   TEXT REFERENCES users(id),
    reason              TEXT NOT NULL DEFAULT '',
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_runner_credentials_runner ON runner_credentials(runner_id);
CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_api_tokens_org_user ON api_tokens(organisation_id, user_id);
CREATE INDEX IF NOT EXISTS idx_automation_schedules_due
    ON automation_schedules(organisation_id, enabled, next_run_at);

-- +goose Down
DROP TABLE IF EXISTS automation_controls;
DROP TABLE IF EXISTS automation_schedules;
DROP TABLE IF EXISTS api_tokens;
ALTER TABLE executions DROP COLUMN IF EXISTS command;
ALTER TABLE runner_credentials DROP COLUMN IF EXISTS runner_id;
