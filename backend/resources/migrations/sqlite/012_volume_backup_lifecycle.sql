-- Source: docklet-next/issues/283
-- Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
-- Retrieved: 2026-10-28
-- License: Apache-2.0
-- Deviations: Original volume backup lifecycle and target foundation.

-- +goose Up
ALTER TABLE volume_backups RENAME TO volume_backups_old;
CREATE TABLE "volume_backups" ("id" TEXT NOT NULL,"created_at" datetime NOT NULL,"updated_at" datetime,"volume_name" TEXT NOT NULL,"size" INTEGER NOT NULL,"status" TEXT NOT NULL DEFAULT 'succeeded',"trigger" TEXT NOT NULL DEFAULT 'manual',"destination" TEXT NOT NULL DEFAULT 'local',"format" TEXT NOT NULL DEFAULT 'archive',"local_snapshot_id" TEXT,"policy_id" TEXT,"error" TEXT,"stop_containers" numeric NOT NULL DEFAULT false,"stopped_containers" TEXT,"error_code" TEXT,"activity_id" TEXT,PRIMARY KEY ("id"),CHECK (stopped_containers IS NULL OR json_valid(stopped_containers)));
INSERT INTO volume_backups ("id","created_at","updated_at","volume_name","size","status","trigger","destination","format","local_snapshot_id","policy_id","error") SELECT "id","created_at","updated_at","volume_name","size","status","trigger","destination","format","local_snapshot_id","policy_id","error" FROM volume_backups_old;
DROP TABLE volume_backups_old;
CREATE INDEX `idx_volume_backups_policy_id` ON `volume_backups`(`policy_id`);
CREATE INDEX `idx_volume_backups_volume_name` ON `volume_backups`(`volume_name`);
CREATE INDEX idx_volume_backups_volume_name_created_at ON volume_backups (volume_name, created_at);

DELETE FROM volume_backup_policies WHERE id IN (
  SELECT id FROM (
    SELECT id, ROW_NUMBER() OVER (
      PARTITION BY volume_name ORDER BY COALESCE(updated_at, created_at) DESC, id DESC
    ) AS position FROM volume_backup_policies
  ) WHERE position > 1
);
CREATE UNIQUE INDEX idx_volume_backup_policies_single_volume ON volume_backup_policies (volume_name);
ALTER TABLE volume_backup_policies ADD COLUMN "target_id" TEXT NOT NULL DEFAULT 'local';

CREATE TABLE "volume_restore_runs" ("id" TEXT NOT NULL,"volume_name" TEXT NOT NULL,"kind" TEXT NOT NULL,"backup_id" TEXT,"safety_backup_id" TEXT,"input_path" TEXT,"input_sha256" TEXT,"phase" TEXT NOT NULL,"status" TEXT NOT NULL,"pending_restart" numeric NOT NULL DEFAULT false,"stopped_containers" TEXT,"error_code" TEXT,"activity_id" TEXT,"started_at" datetime NOT NULL,"finished_at" datetime,"mode" TEXT NOT NULL DEFAULT 'replace',"new_volume_name" TEXT,"created_volume" numeric NOT NULL DEFAULT false,"cleanup_pending" numeric NOT NULL DEFAULT false,PRIMARY KEY ("id"),CHECK (kind IN ('full','files','upload')),CHECK (phase IN ('preparing','writing','done')),CHECK (status IN ('running','succeeded','failed','partial')),CHECK (stopped_containers IS NULL OR json_valid(stopped_containers)),CHECK (mode IN ('replace','new')));
CREATE INDEX `idx_volume_restore_runs_volume_started` ON `volume_restore_runs`(`volume_name`,`started_at`);

CREATE TABLE "backup_targets" ("id" TEXT NOT NULL,"name" TEXT NOT NULL,"kind" TEXT NOT NULL,"host_id" TEXT,"path" TEXT NOT NULL,"server" TEXT NOT NULL,"share" TEXT NOT NULL,"username" TEXT NOT NULL,"secret_enc" TEXT NOT NULL,"options" TEXT NOT NULL,"recovery_key_enc" TEXT NOT NULL,"config_version" INTEGER NOT NULL,"created_at" datetime NOT NULL,"updated_at" datetime,"retired_at" datetime,PRIMARY KEY ("id"),CHECK (kind IN ('local','mount','smb','nfs')),CHECK ((kind IN ('smb','nfs')) = (host_id IS NULL)));
CREATE UNIQUE INDEX idx_backup_targets_active_name ON backup_targets (name) WHERE retired_at IS NULL;
CREATE TABLE "backup_target_checks" ("target_id" TEXT NOT NULL,"host_id" TEXT NOT NULL,"set_version" INTEGER NOT NULL,"config_version" INTEGER NOT NULL,"activity_id" TEXT NOT NULL,"ok" numeric NOT NULL,"code" TEXT,"free_bytes" INTEGER,"total_bytes" INTEGER,"snapshot_count" INTEGER,"at" datetime NOT NULL,PRIMARY KEY ("target_id","host_id"));

DELETE FROM settings WHERE key = 'systemVolumeBackupConfig';
