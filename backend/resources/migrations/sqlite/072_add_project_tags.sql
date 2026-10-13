-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/072_add_project_tags.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
CREATE TABLE project_tags (
    project_id TEXT NOT NULL,
    name TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('ui', 'compose')),
    color TEXT NOT NULL DEFAULT 'gray' CHECK (color IN ('gray', 'purple', 'blue', 'green', 'yellow', 'orange', 'red', 'pink')),
    PRIMARY KEY (project_id, name, source),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE INDEX idx_project_tags_name ON project_tags(name);

-- +goose Down
DROP TABLE IF EXISTS project_tags;
