-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/083_add_container_image_updates.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE image_updates ADD COLUMN container_id TEXT NOT NULL DEFAULT '';
ALTER TABLE image_updates ADD COLUMN image_id TEXT NOT NULL DEFAULT '';
ALTER TABLE image_updates ADD COLUMN project_id TEXT NOT NULL DEFAULT '';
ALTER TABLE image_updates ADD COLUMN service_name TEXT NOT NULL DEFAULT '';
ALTER TABLE image_updates ADD COLUMN policy_key TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_image_updates_container_id ON image_updates(container_id);
CREATE INDEX idx_image_updates_image_id ON image_updates(image_id);
CREATE INDEX idx_image_updates_project_id ON image_updates(project_id);

-- +goose Down
DELETE FROM image_updates WHERE container_id <> '' OR project_id <> '';
DROP INDEX idx_image_updates_project_id;
DROP INDEX idx_image_updates_container_id;
DROP INDEX idx_image_updates_image_id;
ALTER TABLE image_updates DROP COLUMN policy_key;
ALTER TABLE image_updates DROP COLUMN service_name;
ALTER TABLE image_updates DROP COLUMN project_id;
ALTER TABLE image_updates DROP COLUMN image_id;
ALTER TABLE image_updates DROP COLUMN container_id;
