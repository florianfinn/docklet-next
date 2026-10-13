-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/087_add_volume_backup_remote_instance.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE volume_backups ADD COLUMN remote_instance_id text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE volume_backups DROP COLUMN remote_instance_id;
