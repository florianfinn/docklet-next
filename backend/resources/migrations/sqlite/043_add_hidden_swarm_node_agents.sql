-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/043_add_hidden_swarm_node_agents.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE environments ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0;
ALTER TABLE environments ADD COLUMN parent_environment_id TEXT;
ALTER TABLE environments ADD COLUMN swarm_node_id TEXT;

CREATE INDEX IF NOT EXISTS idx_environments_hidden ON environments(hidden);
CREATE INDEX IF NOT EXISTS idx_environments_parent_environment_id ON environments(parent_environment_id);
CREATE INDEX IF NOT EXISTS idx_environments_swarm_node_id ON environments(swarm_node_id);
CREATE INDEX IF NOT EXISTS idx_environments_parent_swarm_node ON environments(parent_environment_id, swarm_node_id);

-- +goose Down
DROP INDEX IF EXISTS idx_environments_parent_swarm_node;
DROP INDEX IF EXISTS idx_environments_swarm_node_id;
DROP INDEX IF EXISTS idx_environments_parent_environment_id;
DROP INDEX IF EXISTS idx_environments_hidden;

ALTER TABLE environments DROP COLUMN swarm_node_id;
ALTER TABLE environments DROP COLUMN parent_environment_id;
ALTER TABLE environments DROP COLUMN hidden;
