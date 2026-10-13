-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/050_add_project_archived.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE projects ADD COLUMN is_archived BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE projects ADD COLUMN archived_at DATETIME;
CREATE INDEX idx_projects_is_archived ON projects(is_archived);

-- +goose Down
DROP INDEX IF EXISTS idx_projects_is_archived;
ALTER TABLE projects DROP COLUMN archived_at;
ALTER TABLE projects DROP COLUMN is_archived;
