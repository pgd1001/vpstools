-- +goose Up
ALTER TABLE audit_events
    ADD COLUMN IF NOT EXISTS previous_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS event_hash TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE audit_events
    DROP COLUMN event_hash,
    DROP COLUMN previous_hash;
