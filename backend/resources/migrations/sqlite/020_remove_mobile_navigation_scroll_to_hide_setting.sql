-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/020_remove_mobile_navigation_scroll_to_hide_setting.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
DELETE FROM settings WHERE key = 'mobileNavigationScrollToHide';

-- +goose Down
INSERT INTO settings (key, value) VALUES ('mobileNavigationScrollToHide', 'true');
