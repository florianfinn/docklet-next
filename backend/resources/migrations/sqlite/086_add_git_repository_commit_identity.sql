-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/086_add_git_repository_commit_identity.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE git_repositories ADD COLUMN commit_author_name TEXT NOT NULL DEFAULT '';
ALTER TABLE git_repositories ADD COLUMN commit_author_email TEXT NOT NULL DEFAULT '';
ALTER TABLE git_repositories ADD COLUMN signing_key TEXT NOT NULL DEFAULT '';
ALTER TABLE git_repositories ADD COLUMN signing_key_passphrase TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE git_repositories DROP COLUMN signing_key_passphrase;
ALTER TABLE git_repositories DROP COLUMN signing_key;
ALTER TABLE git_repositories DROP COLUMN commit_author_email;
ALTER TABLE git_repositories DROP COLUMN commit_author_name;
