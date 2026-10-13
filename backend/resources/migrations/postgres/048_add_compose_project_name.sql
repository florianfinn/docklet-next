-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/048_add_compose_project_name.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE projects ADD COLUMN compose_project_name TEXT;

-- +goose Down
ALTER TABLE projects DROP COLUMN compose_project_name;
