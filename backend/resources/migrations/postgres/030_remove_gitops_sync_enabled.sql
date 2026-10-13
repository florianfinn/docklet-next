-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/030_remove_gitops_sync_enabled.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
-- Remove the enabled column from gitops_syncs table
-- Only autoSync field will control automatic syncing behavior
ALTER TABLE gitops_syncs DROP COLUMN IF EXISTS enabled;

-- +goose Down
-- Re-add the enabled column to gitops_syncs table
ALTER TABLE gitops_syncs ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT true;
CREATE INDEX IF NOT EXISTS idx_gitops_syncs_enabled ON gitops_syncs(enabled);
