-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/014_containers_project_fk.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose NO TRANSACTION
-- +goose Up
PRAGMA foreign_keys=off;
ALTER TABLE containers RENAME COLUMN stack_id TO project_id;
PRAGMA foreign_keys=on;

-- +goose Down
PRAGMA foreign_keys=off;
ALTER TABLE containers RENAME COLUMN project_id TO stack_id;
PRAGMA foreign_keys=on;
