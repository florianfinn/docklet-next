-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/061_add_user_avatar.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE users ADD COLUMN IF NOT EXISTS has_avatar BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS user_avatars (
    user_id TEXT PRIMARY KEY,
    data BYTEA NOT NULL,
    mime_type TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE IF EXISTS user_avatars;
ALTER TABLE users DROP COLUMN IF EXISTS has_avatar;
