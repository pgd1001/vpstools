-- +goose Up
CREATE TABLE runbook_idempotency (
    organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id),
    idempotency_key TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    response_status INTEGER NOT NULL,
    response_body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organisation_id, idempotency_key)
);

CREATE INDEX idx_runbook_idempotency_resource
    ON runbook_idempotency(organisation_id, resource_type, resource_id);

-- +goose Down
DROP TABLE runbook_idempotency;
