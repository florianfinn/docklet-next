-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/059_add_api_key_kind.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'scoped';

-- +goose Down
ALTER TABLE api_keys DROP COLUMN IF EXISTS kind;
