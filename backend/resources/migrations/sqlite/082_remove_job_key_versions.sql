-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/082_remove_job_key_versions.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
UPDATE kv SET "key" = REPLACE("key", 'jobs.v1.', 'jobs.') WHERE "key" LIKE 'jobs.v1.%';
UPDATE kv SET "key" = REPLACE("key", 'jobs-catalog.v1/', 'jobs-catalog/') WHERE "key" LIKE 'jobs-catalog.v1/%';

-- +goose Down
UPDATE kv SET "key" = REPLACE("key", 'jobs.', 'jobs.v1.') WHERE "key" LIKE 'jobs.%';
UPDATE kv SET "key" = REPLACE("key", 'jobs-catalog/', 'jobs-catalog.v1/') WHERE "key" LIKE 'jobs-catalog/%';
