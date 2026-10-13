-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/022_unique_oidc_subject_index.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
-- Ensure OIDC subject is unique across users while allowing NULL values (SQLite >= 3.8 supports partial indexes)
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_oidc_subject_id_unique
ON users(oidc_subject_id)
WHERE oidc_subject_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_users_oidc_subject_id_unique;
