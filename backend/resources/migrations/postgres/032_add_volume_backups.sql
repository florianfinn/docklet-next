-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/032_add_volume_backups.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
CREATE TABLE volume_backups (
    id TEXT PRIMARY KEY,
    volume_name TEXT NOT NULL,
    size BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE
);
CREATE INDEX idx_volume_backups_volume_name ON volume_backups(volume_name);

-- +goose Down
DROP TABLE volume_backups;
