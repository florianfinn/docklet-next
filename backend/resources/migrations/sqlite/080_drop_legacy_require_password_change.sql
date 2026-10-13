-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/080_drop_legacy_require_password_change.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE users DROP COLUMN require_password_change;

-- +goose Down
ALTER TABLE users ADD COLUMN require_password_change BOOLEAN NOT NULL DEFAULT 0;
