-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/057_add_user_font_size.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE users ADD COLUMN IF NOT EXISTS font_size INTEGER;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS font_size;
