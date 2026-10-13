-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/010_rework_environments.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE environments DROP COLUMN IF EXISTS hostname;
ALTER TABLE environments DROP COLUMN IF EXISTS description;
ALTER TABLE environments ADD COLUMN IF NOT EXISTS access_token TEXT;
CREATE INDEX IF NOT EXISTS idx_environments_api_url ON environments (api_url);

-- +goose Down
ALTER TABLE environments DROP COLUMN IF EXISTS access_token;
ALTER TABLE environments ADD COLUMN IF NOT EXISTS hostname TEXT NOT NULL DEFAULT '';
ALTER TABLE environments ADD COLUMN IF NOT EXISTS description TEXT;
DROP INDEX IF EXISTS idx_environments_api_url;
