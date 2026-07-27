-- +goose Up
ALTER TABLE audit_events
    ADD COLUMN previous_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN event_hash TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE audit_events
    DROP COLUMN event_hash,
    DROP COLUMN previous_hash;
