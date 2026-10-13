-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/074_add_gitops_sync_pull_redeploy.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
ALTER TABLE gitops_syncs ADD COLUMN pull_image_after_sync BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE gitops_syncs ADD COLUMN redeploy_after_sync BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE gitops_syncs DROP COLUMN IF EXISTS redeploy_after_sync;
ALTER TABLE gitops_syncs DROP COLUMN IF EXISTS pull_image_after_sync;
