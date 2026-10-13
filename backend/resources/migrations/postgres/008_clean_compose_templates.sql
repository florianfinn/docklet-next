-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/008_clean_compose_templates.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE compose_templates 
  DROP COLUMN IF EXISTS meta_updated_at;

-- +goose Down
ALTER TABLE compose_templates
  ADD COLUMN IF NOT EXISTS meta_updated_at TEXT;
