-- +goose Up
CREATE TABLE execution_result_receipts (
    organisation_id TEXT NOT NULL REFERENCES organisations(id),
    execution_id TEXT NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES execution_targets(id) ON DELETE CASCADE,
    lease_id TEXT NOT NULL,
    runner_id TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    response_code INTEGER NOT NULL DEFAULT 200,
    response_body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (target_id, lease_id)
);

CREATE INDEX idx_result_receipts_execution
    ON execution_result_receipts(organisation_id, execution_id);

-- +goose Down
DROP TABLE execution_result_receipts;
