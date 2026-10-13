-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/009_add_image_update_auth_fields.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE image_updates ADD COLUMN auth_method TEXT;
ALTER TABLE image_updates ADD COLUMN auth_username TEXT;
ALTER TABLE image_updates ADD COLUMN auth_registry TEXT;
ALTER TABLE image_updates ADD COLUMN used_credential INTEGER DEFAULT 0;

-- +goose Down
ALTER TABLE image_updates DROP COLUMN auth_method;
ALTER TABLE image_updates DROP COLUMN auth_username;
ALTER TABLE image_updates DROP COLUMN auth_registry;
ALTER TABLE image_updates DROP COLUMN used_credential;
