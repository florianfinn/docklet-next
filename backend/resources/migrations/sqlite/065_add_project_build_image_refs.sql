-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/065_add_project_build_image_refs.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE projects ADD COLUMN build_image_refs_json TEXT;

-- +goose Down
ALTER TABLE projects DROP COLUMN build_image_refs_json;
