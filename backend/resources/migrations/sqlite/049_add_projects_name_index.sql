-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/049_add_projects_name_index.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
CREATE INDEX IF NOT EXISTS idx_projects_name ON projects (name);

-- +goose Down
DROP INDEX IF EXISTS idx_projects_name;
