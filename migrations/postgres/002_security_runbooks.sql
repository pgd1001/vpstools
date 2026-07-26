-- +goose Up

ALTER TABLE executions ADD COLUMN IF NOT EXISTS delegated_by_user_id TEXT REFERENCES users(id);
ALTER TABLE executions ADD COLUMN IF NOT EXISTS approval_id TEXT;
ALTER TABLE execution_targets ADD COLUMN IF NOT EXISTS stdout TEXT NOT NULL DEFAULT '';
ALTER TABLE execution_targets ADD COLUMN IF NOT EXISTS stderr TEXT NOT NULL DEFAULT '';
ALTER TABLE execution_targets ADD COLUMN IF NOT EXISTS stdout_artifact_id TEXT;
ALTER TABLE execution_targets ADD COLUMN IF NOT EXISTS stderr_artifact_id TEXT;
ALTER TABLE execution_targets ADD COLUMN IF NOT EXISTS lease_id TEXT;
ALTER TABLE execution_targets ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;
ALTER TABLE execution_targets ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 0;
ALTER TABLE execution_targets ADD COLUMN IF NOT EXISTS max_attempts INTEGER NOT NULL DEFAULT 3;
ALTER TABLE execution_targets ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS execution_events (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    execution_id TEXT NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
    target_id TEXT REFERENCES execution_targets(id) ON DELETE CASCADE,
    from_status TEXT,
    to_status TEXT NOT NULL,
    event_type TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS artifact_records (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    owner_type TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    content_type TEXT NOT NULL,
    byte_size BIGINT NOT NULL DEFAULT 0,
    sha256 TEXT NOT NULL,
    backend TEXT NOT NULL DEFAULT 'local',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS runner_credentials (
    id              TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    token_hash      TEXT NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS runbooks (
    id              TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    name            TEXT NOT NULL,
    title           TEXT NOT NULL,
    description     TEXT,
    status          TEXT NOT NULL DEFAULT 'draft',
    current_version_id TEXT,
    created_by_user_id TEXT NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT runbooks_unique_name_per_org UNIQUE (organisation_id, name)
);

CREATE TABLE IF NOT EXISTS runbook_versions (
    id              TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    runbook_id      TEXT NOT NULL REFERENCES runbooks(id) ON DELETE CASCADE,
    version         INTEGER NOT NULL,
    status          TEXT NOT NULL DEFAULT 'draft',
    risk_level      TEXT NOT NULL DEFAULT 'medium',
    definition_yaml TEXT NOT NULL,
    definition_json TEXT NOT NULL,
    parameter_schema TEXT NOT NULL DEFAULT '{}',
    target_constraints TEXT NOT NULL DEFAULT '{}',
    approval_rules  TEXT NOT NULL DEFAULT '{}',
    allowed_roles   TEXT NOT NULL DEFAULT '["senior_engineer","admin","owner"]',
    command_preview TEXT,
    command_hash    TEXT,
    created_by_user_id TEXT NOT NULL REFERENCES users(id),
    published_by_user_id TEXT REFERENCES users(id),
    published_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT runbook_versions_unique UNIQUE (runbook_id, version)
);

CREATE TABLE IF NOT EXISTS approval_requests (
    id              TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    requester_user_id TEXT NOT NULL REFERENCES users(id),
    approver_user_id TEXT REFERENCES users(id),
    action_type     TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    risk_level      TEXT NOT NULL DEFAULT 'medium',
    reason          TEXT NOT NULL,
    target_type     TEXT NOT NULL,
    target_id       TEXT,
    target_snapshot TEXT NOT NULL DEFAULT '{}',
    request_payload TEXT NOT NULL DEFAULT '{}',
    expires_at      TIMESTAMPTZ NOT NULL,
    decided_at      TIMESTAMPTZ,
    decision_note   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_runner_credentials_hash ON runner_credentials(token_hash);

-- +goose Down

DROP TABLE IF EXISTS approval_requests;
DROP TABLE IF EXISTS runbook_versions;
DROP TABLE IF EXISTS runbooks;
DROP TABLE IF EXISTS runner_credentials;
