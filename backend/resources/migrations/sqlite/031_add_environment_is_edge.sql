-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/031_add_environment_is_edge.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
-- add is_edge column to environments table for edge agent mode
ALTER TABLE environments ADD COLUMN is_edge INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- remove is_edge column from environments table
ALTER TABLE environments DROP COLUMN is_edge;
