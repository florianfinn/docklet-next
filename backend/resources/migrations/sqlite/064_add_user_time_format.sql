-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/064_add_user_time_format.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE users ADD COLUMN time_format TEXT NOT NULL DEFAULT 'auto'
    CHECK (time_format IN ('auto', '12h', '24h'));

-- +goose Down
ALTER TABLE users DROP COLUMN time_format;
