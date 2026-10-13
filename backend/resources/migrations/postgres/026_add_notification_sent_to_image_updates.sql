-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/026_add_notification_sent_to_image_updates.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
-- Add notification_sent column to track if updates have been sent
-- Existing indexes on repository and tag are preserved automatically
ALTER TABLE IF EXISTS image_updates
    ADD COLUMN notification_sent BOOLEAN DEFAULT false;

-- +goose Down
-- Remove notification_sent column
-- PostgreSQL automatically preserves existing indexes on other columns
ALTER TABLE IF EXISTS image_updates
    DROP COLUMN notification_sent;
