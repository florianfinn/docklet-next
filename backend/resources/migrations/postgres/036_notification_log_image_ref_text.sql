-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/036_notification_log_image_ref_text.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
-- Change image_ref from VARCHAR(255) to TEXT to support batch notifications with multiple image refs
ALTER TABLE notification_logs ALTER COLUMN image_ref TYPE TEXT;

-- +goose Down
-- Revert image_ref back to VARCHAR(255)
ALTER TABLE notification_logs ALTER COLUMN image_ref TYPE VARCHAR(255);
