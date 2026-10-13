-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/012_add_user_locale.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE users ADD COLUMN IF NOT EXISTS locale TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS locale;
