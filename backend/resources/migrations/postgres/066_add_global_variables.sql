-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/066_add_global_variables.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
CREATE TABLE global_variables (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ,
    key TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    is_secret BOOLEAN NOT NULL DEFAULT FALSE,
    all_environments BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE INDEX idx_global_variables_key ON global_variables(key);

CREATE TABLE global_variable_environments (
    global_variable_id TEXT NOT NULL REFERENCES global_variables(id) ON DELETE CASCADE,
    environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    PRIMARY KEY (global_variable_id, environment_id)
);

CREATE INDEX idx_global_variable_environments_env ON global_variable_environments(environment_id);

-- +goose Down
DROP TABLE IF EXISTS global_variable_environments;
DROP TABLE IF EXISTS global_variables;
