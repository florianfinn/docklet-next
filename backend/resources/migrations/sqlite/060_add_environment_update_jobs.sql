-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/060_add_environment_update_jobs.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
CREATE TABLE environment_update_jobs (
    id TEXT PRIMARY KEY,
    created_at DATETIME NOT NULL,
    updated_at DATETIME,
    status TEXT NOT NULL,
    user_id TEXT,
    username TEXT,
    manager_version_at_start TEXT,
    manager_digest_at_start TEXT,
    manager_target_version TEXT,
    results TEXT,
    error TEXT,
    completed_at DATETIME
);

CREATE INDEX idx_environment_update_jobs_status ON environment_update_jobs(status);

-- +goose Down
DROP TABLE IF EXISTS environment_update_jobs;
