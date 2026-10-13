-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/023_add_apprise_settings.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
CREATE TABLE IF NOT EXISTS apprise_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    api_url TEXT NOT NULL,
    enabled INTEGER DEFAULT 0,
    image_update_tag TEXT,
    container_update_tag TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS apprise_settings;
