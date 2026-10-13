-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/009_add_image_update_auth_fields.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE IF EXISTS image_updates
    ADD COLUMN IF NOT EXISTS auth_method TEXT,
    ADD COLUMN IF NOT EXISTS auth_username TEXT,
    ADD COLUMN IF NOT EXISTS auth_registry TEXT,
    ADD COLUMN IF NOT EXISTS used_credential BOOLEAN DEFAULT FALSE;

-- +goose Down
ALTER TABLE IF EXISTS image_updates
    DROP COLUMN IF EXISTS used_credential,
    DROP COLUMN IF EXISTS auth_registry,
    DROP COLUMN IF EXISTS auth_username,
    DROP COLUMN IF EXISTS auth_method;
