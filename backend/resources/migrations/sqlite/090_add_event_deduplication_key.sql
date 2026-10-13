-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/090_add_event_deduplication_key.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE events ADD COLUMN deduplication_key TEXT;
CREATE UNIQUE INDEX idx_events_deduplication_key ON events (deduplication_key);

-- +goose Down
DROP INDEX idx_events_deduplication_key;
ALTER TABLE events DROP COLUMN deduplication_key;
