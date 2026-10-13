-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/007_add_user_oidc_tokens.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE users ADD COLUMN oidc_access_token TEXT;
ALTER TABLE users ADD COLUMN oidc_refresh_token TEXT;
ALTER TABLE users ADD COLUMN oidc_access_token_expires_at DATETIME;

-- +goose Down
ALTER TABLE users DROP COLUMN oidc_access_token;
ALTER TABLE users DROP COLUMN oidc_refresh_token;
ALTER TABLE users DROP COLUMN oidc_access_token_expires_at;
