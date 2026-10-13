-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/069_add_container_registry_repository_names.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE container_registries ADD COLUMN IF NOT EXISTS repository_names TEXT NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE container_registries DROP COLUMN IF EXISTS repository_names;
