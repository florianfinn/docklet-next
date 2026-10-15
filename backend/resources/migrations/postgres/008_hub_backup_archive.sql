-- Source: docklet-next/docs/design/hub-backup.md
-- Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
-- Retrieved: 2026-10-15
-- License: Apache-2.0
-- Deviations: Original forward migration for issue #32; retains the volume recovery-key store.
-- +goose Up
-- Arcane system snapshots cannot be read as PostgreSQL archives.
DELETE FROM system_backup_runs;
UPDATE system_backup_policies SET enabled = false;

ALTER TABLE system_backup_runs
    DROP COLUMN destination,
    DROP COLUMN local_snapshot_id,
    DROP COLUMN error,
    ADD COLUMN archive_name text,
    ADD COLUMN activity_id text,
    ADD COLUMN completed_at timestamptz,
    ADD COLUMN source_server_major bigint,
    ADD COLUMN migration_version bigint,
    ADD COLUMN copy_target_id text,
    ADD COLUMN copy_state text NOT NULL DEFAULT 'none',
    ADD COLUMN copy_error_code text,
    ADD COLUMN error_code text,
    ADD CONSTRAINT system_backup_runs_size_limit CHECK (size BETWEEN 0 AND 2147483648),
    ADD CONSTRAINT system_backup_runs_archive_name CHECK (archive_name IS NULL OR archive_name ~ '^[A-Za-z0-9_-]+[.]dump$'),
    ADD CONSTRAINT system_backup_runs_server_major CHECK (source_server_major IS NULL OR source_server_major = 18),
    ADD CONSTRAINT system_backup_runs_migration_version CHECK (migration_version IS NULL OR migration_version >= 1),
    ADD CONSTRAINT system_backup_runs_copy_state CHECK (copy_state IN ('none', 'pending', 'succeeded', 'failed'));
CREATE UNIQUE INDEX idx_system_backup_runs_archive_name ON system_backup_runs (archive_name) WHERE archive_name IS NOT NULL;
CREATE INDEX idx_system_backup_runs_activity_id ON system_backup_runs (activity_id) WHERE activity_id IS NOT NULL;

ALTER TABLE system_backup_policies
    DROP COLUMN local_enabled,
    ADD COLUMN target_id text,
    ADD CONSTRAINT system_backup_policies_retention_count CHECK (retention_count >= 1);
