-- +goose Up

CREATE TABLE IF NOT EXISTS ai_requests (
    id                  TEXT PRIMARY KEY,
    organisation_id     TEXT NOT NULL REFERENCES organisations(id),
    actor_user_id       TEXT NOT NULL REFERENCES users(id),
    status              TEXT NOT NULL,
    request_json        JSONB NOT NULL DEFAULT '{}',
    response_text       TEXT NOT NULL DEFAULT '',
    model               TEXT NOT NULL DEFAULT '',
    provider_request_id TEXT NOT NULL DEFAULT '',
    duration_ms         INTEGER NOT NULL DEFAULT 0,
    error_summary       TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ai_evidence (
    id              TEXT PRIMARY KEY,
    request_id      TEXT NOT NULL REFERENCES ai_requests(id) ON DELETE CASCADE,
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    ordinal         INTEGER NOT NULL,
    kind            TEXT NOT NULL,
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    source_uri      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_requests_org_time
    ON ai_requests(organisation_id, created_at);
CREATE INDEX IF NOT EXISTS idx_ai_evidence_request
    ON ai_evidence(request_id, ordinal);

-- +goose Down

DROP TABLE IF EXISTS ai_evidence;
DROP TABLE IF EXISTS ai_requests;
