-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/008_clean_compose_templates.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE compose_templates DROP COLUMN meta_updated_at;

-- +goose Down
ALTER TABLE compose_templates ADD COLUMN meta_updated_at TEXT;
