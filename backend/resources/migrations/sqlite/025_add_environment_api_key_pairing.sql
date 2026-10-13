-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/025_add_environment_api_key_pairing.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
-- Add api_key_id column to environments table for API key-based pairing
ALTER TABLE environments ADD COLUMN api_key_id TEXT REFERENCES api_keys(id) ON DELETE SET NULL;

-- Add environment_id column to api_keys table to link API keys to environments
ALTER TABLE api_keys ADD COLUMN environment_id TEXT REFERENCES environments(id) ON DELETE CASCADE;

-- +goose Down
-- Remove environment_id column from api_keys table
ALTER TABLE api_keys DROP COLUMN environment_id;

-- Remove api_key_id column from environments table
ALTER TABLE environments DROP COLUMN api_key_id;
