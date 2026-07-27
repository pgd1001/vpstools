-- +goose Up
CREATE TABLE execution_idempotency (
    organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id),
    idempotency_key TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    execution_id TEXT NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
    response_body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organisation_id, idempotency_key)
);

CREATE INDEX idx_execution_idempotency_execution
    ON execution_idempotency(organisation_id, execution_id);

-- +goose Down
DROP TABLE execution_idempotency;
