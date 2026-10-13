-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/011_add_environment_name.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE environments ADD COLUMN IF NOT EXISTS name TEXT;
CREATE INDEX IF NOT EXISTS idx_environments_name ON environments (name);

-- +goose Down
ALTER TABLE environments DROP COLUMN IF EXISTS name;
DROP INDEX IF EXISTS idx_environments_name;
