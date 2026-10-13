-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/047_add_gitops_sync_target_type.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE gitops_syncs ADD COLUMN target_type VARCHAR(255) NOT NULL DEFAULT 'project';

-- +goose Down
ALTER TABLE gitops_syncs DROP COLUMN IF EXISTS target_type;
