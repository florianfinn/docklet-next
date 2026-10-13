-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/058_add_environment_last_edge_transport.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE environments ADD COLUMN last_edge_transport TEXT;

-- +goose Down
ALTER TABLE environments DROP COLUMN last_edge_transport;
