-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/036_notification_log_image_ref_text.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
-- SQLite doesn't enforce VARCHAR length, but we recreate the table for consistency
-- SQLite treats VARCHAR(255) and TEXT the same, so this is a no-op for existing data
-- but documents the intended schema change

-- +goose Down
-- No-op: SQLite doesn't enforce VARCHAR length constraints
