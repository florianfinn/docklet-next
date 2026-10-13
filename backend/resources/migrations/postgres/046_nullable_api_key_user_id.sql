-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/046_nullable_api_key_user_id.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
-- Environment bootstrap keys are owned by the system, not a user.
-- Allow user_id to be NULL so agent-side key creation doesn't violate the FK constraint.
ALTER TABLE api_keys ALTER COLUMN user_id DROP NOT NULL;

-- +goose Down
-- Remove environment bootstrap keys (user_id IS NULL) before restoring the NOT NULL constraint.
DELETE FROM api_keys WHERE user_id IS NULL;
ALTER TABLE api_keys ALTER COLUMN user_id SET NOT NULL;
