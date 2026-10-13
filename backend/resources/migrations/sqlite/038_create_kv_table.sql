-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/038_create_kv_table.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
CREATE TABLE IF NOT EXISTS kv (
    key TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME
);

-- +goose Down
DROP TABLE IF EXISTS kv;
