-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/067_add_activity_batch_id.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE activities ADD COLUMN batch_id TEXT;

CREATE INDEX idx_activities_environment_batch ON activities(environment_id, batch_id);

-- +goose Down
DROP INDEX IF EXISTS idx_activities_environment_batch;
ALTER TABLE activities DROP COLUMN batch_id;
