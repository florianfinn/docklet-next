-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/084_route_job_activities_to_environment.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
UPDATE activities
SET environment_id = CAST(metadata AS jsonb)->>'environmentId'
WHERE type = 'job_run'
  AND jsonb_typeof(CAST(metadata AS jsonb)->'environmentId') = 'string'
  AND CAST(metadata AS jsonb)->>'environmentId' <> '';

-- +goose Down
UPDATE activities SET environment_id = '0' WHERE type = 'job_run';
