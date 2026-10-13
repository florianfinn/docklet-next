-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/007_add_user_oidc_tokens.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE IF EXISTS users
  ADD COLUMN IF NOT EXISTS oidc_access_token TEXT,
  ADD COLUMN IF NOT EXISTS oidc_refresh_token TEXT,
  ADD COLUMN IF NOT EXISTS oidc_access_token_expires_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE IF EXISTS users
  DROP COLUMN IF EXISTS oidc_access_token,
  DROP COLUMN IF EXISTS oidc_refresh_token,
  DROP COLUMN IF EXISTS oidc_access_token_expires_at;
