-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/023_add_apprise_settings.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
CREATE TABLE IF NOT EXISTS apprise_settings (
    id SERIAL PRIMARY KEY,
    api_url VARCHAR(512) NOT NULL,
    enabled BOOLEAN DEFAULT FALSE,
    image_update_tag VARCHAR(255),
    container_update_tag VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS apprise_settings;
