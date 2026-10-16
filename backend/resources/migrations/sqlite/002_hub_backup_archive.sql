-- Source: docklet-next/docs/design/hub-backup.md
-- Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
-- Retrieved: 2026-10-16
-- License: Apache-2.0
-- Deviations: Forward SQLite archive metadata; keeps the volume recovery-key store.
-- +goose Up
DROP TABLE system_backup_runs;
ALTER TABLE system_backup_policies RENAME TO previous_system_backup_policies;
CREATE TABLE "system_backup_policies" ("id" TEXT NOT NULL,"created_at" datetime NOT NULL DEFAULT (CAST((julianday('now') - 2440587.5) * 86400000000 AS INTEGER)),"updated_at" datetime,"enabled" numeric NOT NULL DEFAULT false,"schedule" TEXT NOT NULL,"retention_count" INTEGER NOT NULL DEFAULT 7,"target_id" TEXT,PRIMARY KEY ("id"),CHECK (retention_count >= 1));
CREATE TABLE "system_backup_runs" ("id" TEXT NOT NULL,"created_at" datetime NOT NULL DEFAULT (CAST((julianday('now') - 2440587.5) * 86400000000 AS INTEGER)),"updated_at" datetime,"size" INTEGER NOT NULL DEFAULT 0,"status" TEXT NOT NULL,"trigger" TEXT NOT NULL,"archive_name" TEXT,"activity_id" TEXT,"completed_at" datetime,"archive_version" INTEGER,"migration_version" INTEGER,"copy_target_id" TEXT,"copy_state" TEXT NOT NULL DEFAULT 'none',"copy_error_code" TEXT,"error_code" TEXT,"policy_id" TEXT,PRIMARY KEY ("id"),CHECK (size BETWEEN 0 AND 2147483648),CHECK (archive_name IS NULL OR (length(archive_name) BETWEEN 5 AND 132 AND archive_name NOT GLOB '*[^A-Za-z0-9_.-]*' AND substr(archive_name,-4) = '.tar')),CHECK (archive_version IS NULL OR archive_version = 1),CHECK (migration_version IS NULL OR migration_version >= 1),CHECK (copy_state IN ('none','pending','succeeded','failed')));
INSERT INTO system_backup_policies (id,created_at,updated_at,enabled,schedule,retention_count) SELECT id,created_at,updated_at,false,schedule,max(1,retention_count) FROM previous_system_backup_policies;
DROP TABLE previous_system_backup_policies;
CREATE INDEX idx_system_backup_runs_activity_id ON system_backup_runs (activity_id) WHERE activity_id IS NOT NULL;
CREATE UNIQUE INDEX idx_system_backup_runs_archive_name ON system_backup_runs (archive_name) WHERE archive_name IS NOT NULL;
CREATE INDEX `idx_system_backup_runs_policy_id` ON `system_backup_runs`(`policy_id`);
