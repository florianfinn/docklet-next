-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/016_drop_image_table.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
DROP TABLE IF EXISTS images_table;

-- +goose Down
CREATE TABLE IF NOT EXISTS images_table (
    id TEXT PRIMARY KEY,
    repo_tags TEXT,
    repo_digests TEXT,
    size INTEGER NOT NULL,
    virtual_size INTEGER NOT NULL DEFAULT 0,
    labels TEXT,
    created DATETIME NOT NULL,
    repo TEXT,
    tag TEXT,
    in_use BOOLEAN NOT NULL DEFAULT false,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME
);
