-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/026_add_notification_sent_to_image_updates.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
-- Add notification_sent column to track if updates have been sent
-- Existing indexes on repository and tag are preserved automatically
ALTER TABLE image_updates ADD COLUMN notification_sent INTEGER DEFAULT 0;

-- +goose Down
-- SQLite doesn't support DROP COLUMN directly, so we need to recreate the table
-- Create backup without notification_sent column
CREATE TABLE image_updates_backup AS SELECT 
    id, repository, tag, has_update, update_type, current_version, 
    latest_version, current_digest, latest_digest, check_time, 
    response_time_ms, last_error, auth_method, auth_username, 
    auth_registry, used_credential, created_at, updated_at
FROM image_updates;

DROP TABLE image_updates;

ALTER TABLE image_updates_backup RENAME TO image_updates;

-- Recreate indexes that were lost during table recreation
CREATE INDEX IF NOT EXISTS idx_image_update_repository ON image_updates(repository);
CREATE INDEX IF NOT EXISTS idx_image_update_tag ON image_updates(tag);
