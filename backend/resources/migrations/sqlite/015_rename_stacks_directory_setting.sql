-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/015_rename_stacks_directory_setting.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
INSERT OR IGNORE INTO settings (key, value)
SELECT 'projectsDirectory', value
FROM settings
WHERE key = 'stacksDirectory';

DELETE FROM settings WHERE key = 'stacksDirectory';

-- +goose Down
INSERT OR IGNORE INTO settings (key, value)
SELECT 'stacksDirectory', value
FROM settings
WHERE key = 'projectsDirectory';

DELETE FROM settings WHERE key = 'projectsDirectory';
