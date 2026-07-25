-- +goose Up
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE organisations (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    status          TEXT NOT NULL DEFAULT 'active',
    settings        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id              TEXT PRIMARY KEY,
    email           TEXT NOT NULL UNIQUE,
    display_name    TEXT NOT NULL,
    user_type       TEXT NOT NULL DEFAULT 'human',
    status          TEXT NOT NULL DEFAULT 'active',
    external_subject TEXT,
    external_provider TEXT,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
    id              TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    user_id         TEXT NOT NULL REFERENCES users(id),
    role            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT memberships_unique_user_org UNIQUE (organisation_id, user_id)
);

CREATE TABLE servers (
    id              TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    name            TEXT NOT NULL,
    hostname        TEXT,
    public_ip       INET,
    private_ip      INET,
    ssh_port        INTEGER NOT NULL DEFAULT 22,
    ssh_username    TEXT,
    environment     TEXT NOT NULL DEFAULT 'development',
    provider        TEXT,
    os_name         TEXT,
    os_version      TEXT,
    kernel_version  TEXT,
    architecture    TEXT,
    status          TEXT NOT NULL DEFAULT 'pending',
    last_seen_at    TIMESTAMPTZ,
    last_check_at   TIMESTAMPTZ,
    archived_at     TIMESTAMPTZ,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT servers_unique_name_per_org UNIQUE (organisation_id, name)
);

CREATE TABLE server_tags (
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    server_id       TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    key             TEXT NOT NULL,
    value           TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organisation_id, server_id, key)
);

CREATE TABLE runners (
    id              TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    name            TEXT NOT NULL,
    runner_type     TEXT NOT NULL DEFAULT 'customer_managed',
    status          TEXT NOT NULL DEFAULT 'pending',
    version         TEXT,
    hostname        TEXT,
    platform        TEXT,
    ip_address      INET,
    last_seen_at    TIMESTAMPTZ,
    registered_at   TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT runners_unique_name_per_org UNIQUE (organisation_id, name)
);

CREATE TABLE runner_scopes (
    id              TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    runner_id       TEXT NOT NULL REFERENCES runners(id) ON DELETE CASCADE,
    scope_type      TEXT NOT NULL,
    scope_value     TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT runner_scopes_unique UNIQUE (runner_id, scope_type, scope_value)
);

CREATE TABLE runner_credentials (
    id              TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    token_hash      TEXT NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE executions (
    id              TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    actor_user_id   TEXT NOT NULL REFERENCES users(id),
    actor_role_at_time TEXT NOT NULL,
    delegated_by_user_id TEXT REFERENCES users(id),
    approval_id     TEXT,
    execution_type  TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'created',
    risk_level      TEXT NOT NULL DEFAULT 'medium',
    environment     TEXT,
    reason          TEXT,
    command_preview TEXT,
    command_hash    TEXT,
    timeout_seconds INTEGER NOT NULL DEFAULT 300,
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    queued_at       TIMESTAMPTZ,
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    error_summary   TEXT,
    metadata        JSONB NOT NULL DEFAULT '{}'
);

CREATE TABLE runbooks (
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

CREATE TABLE runbook_versions (
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

CREATE TABLE approval_requests (
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

CREATE TABLE execution_targets (
    id              TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    execution_id    TEXT NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
    server_id       TEXT NOT NULL REFERENCES servers(id),
    runner_id       TEXT REFERENCES runners(id),
    status          TEXT NOT NULL DEFAULT 'pending',
    server_snapshot JSONB NOT NULL DEFAULT '{}',
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    exit_code       INTEGER,
    stdout_bytes    INTEGER NOT NULL DEFAULT 0,
    stderr_bytes    INTEGER NOT NULL DEFAULT 0,
    error_summary   TEXT,
    stdout          TEXT NOT NULL DEFAULT '',
    stderr          TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT execution_targets_unique UNIQUE (execution_id, server_id)
);

CREATE TABLE audit_events (
    id              TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_type      TEXT NOT NULL DEFAULT 'user',
    actor_user_id   TEXT REFERENCES users(id),
    actor_display   TEXT,
    actor_role_at_time TEXT,
    action          TEXT NOT NULL,
    target_type     TEXT,
    target_id       TEXT,
    target_display  TEXT,
    environment     TEXT,
    result          TEXT NOT NULL,
    reason          TEXT,
    command_hash    TEXT,
    command_preview TEXT,
    metadata        JSONB NOT NULL DEFAULT '{}'
);

-- Indexes
CREATE INDEX idx_organisations_slug ON organisations(slug);
CREATE INDEX idx_users_email_lower ON users(lower(email));
CREATE INDEX idx_memberships_user ON memberships(user_id);
CREATE INDEX idx_memberships_org_role ON memberships(organisation_id, role);
CREATE INDEX idx_servers_org_status ON servers(organisation_id, status);
CREATE INDEX idx_servers_org_env ON servers(organisation_id, environment);
CREATE INDEX idx_server_tags_org_kv ON server_tags(organisation_id, key, value);
CREATE INDEX idx_server_tags_server ON server_tags(server_id);
CREATE INDEX idx_runners_org_status ON runners(organisation_id, status);
CREATE INDEX idx_runners_last_seen ON runners(organisation_id, last_seen_at DESC);
CREATE INDEX idx_runner_scopes_runner ON runner_scopes(runner_id);
CREATE INDEX idx_runner_credentials_hash ON runner_credentials(token_hash);
CREATE INDEX idx_executions_org_status ON executions(organisation_id, status);
CREATE INDEX idx_executions_actor ON executions(actor_user_id, requested_at DESC);
CREATE INDEX idx_execution_targets_exec ON execution_targets(execution_id);
CREATE INDEX idx_execution_targets_server ON execution_targets(server_id, created_at DESC);
CREATE INDEX idx_audit_events_org_time ON audit_events(organisation_id, occurred_at DESC);
CREATE INDEX idx_audit_events_actor ON audit_events(organisation_id, actor_user_id, occurred_at DESC);
CREATE INDEX idx_audit_events_action ON audit_events(organisation_id, action, occurred_at DESC);
CREATE INDEX idx_audit_events_target ON audit_events(organisation_id, target_type, target_id, occurred_at DESC);

-- +goose Down
DROP TABLE IF EXISTS execution_targets CASCADE;
DROP TABLE IF EXISTS executions CASCADE;
DROP TABLE IF EXISTS runner_scopes CASCADE;
DROP TABLE IF EXISTS runners CASCADE;
DROP TABLE IF EXISTS server_tags CASCADE;
DROP TABLE IF EXISTS servers CASCADE;
DROP TABLE IF EXISTS memberships CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS organisations CASCADE;
DROP TABLE IF EXISTS audit_events CASCADE;
