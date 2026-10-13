-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/016_drop_image_table.sql
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
    size BIGINT NOT NULL,
    virtual_size BIGINT NOT NULL DEFAULT 0,
    labels TEXT,
    created TIMESTAMP NOT NULL,
    repo TEXT,
    tag TEXT,
    in_use BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP
);
