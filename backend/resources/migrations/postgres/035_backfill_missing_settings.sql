-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/035_backfill_missing_settings.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
-- Backfill missing vulnerability scan settings defaults
INSERT INTO settings (key, value)
VALUES ('vulnerabilityScanEnabled', 'false')
ON CONFLICT (key) DO NOTHING;

UPDATE settings
SET value = 'false'
WHERE key = 'vulnerabilityScanEnabled'
  AND (value IS NULL OR btrim(value) = '');

INSERT INTO settings (key, value)
VALUES ('vulnerabilityScanInterval', '0 0 0 * * *')
ON CONFLICT (key) DO NOTHING;

UPDATE settings
SET value = '0 0 0 * * *'
WHERE key = 'vulnerabilityScanInterval'
  AND (value IS NULL OR btrim(value) = '');

-- Backfill missing auto-update excluded containers setting
INSERT INTO settings (key, value)
VALUES ('autoUpdateExcludedContainers', '')
ON CONFLICT (key) DO NOTHING;

UPDATE settings
SET value = ''
WHERE key = 'autoUpdateExcludedContainers'
  AND value IS NULL;

-- +goose Down
-- No-op: data backfill migration cannot be safely reversed.
