-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/003_remove_events_environment_fk.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_environment_id_fkey;

-- +goose Down
ALTER TABLE events ADD CONSTRAINT events_environment_id_fkey 
    FOREIGN KEY (environment_id) REFERENCES environments(id) ON DELETE SET NULL;
