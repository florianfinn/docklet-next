-- Source: docklet-next/issues/335
-- Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
-- Retrieved: 2026-11-03
-- License: Apache-2.0
-- Deviations: Original runtime history, stop intent and personal preference foundation.

-- +goose Up
CREATE TABLE "history_buckets" ("kind" TEXT NOT NULL CHECK (kind IN ('host','container','stack')),"host_id" TEXT NOT NULL,"entity_key" TEXT NOT NULL,"tier" INTEGER NOT NULL CHECK (tier IN (15,120,480)),"bucket_start" INTEGER NOT NULL,"last_minute" INTEGER NOT NULL,"minutes_running" INTEGER NOT NULL DEFAULT 0,"minutes_unhealthy" INTEGER NOT NULL DEFAULT 0,"minutes_down" INTEGER NOT NULL DEFAULT 0,"minutes_stopped" INTEGER NOT NULL DEFAULT 0,"cpu_sum" REAL NOT NULL DEFAULT 0,"cpu_n" INTEGER NOT NULL DEFAULT 0,"cpu_max" REAL NOT NULL DEFAULT 0,"mem_sum" REAL NOT NULL DEFAULT 0,"mem_n" INTEGER NOT NULL DEFAULT 0,"mem_max" REAL NOT NULL DEFAULT 0,"mem_limit_max" REAL NOT NULL DEFAULT 0,"net_rx" REAL NOT NULL DEFAULT 0,"net_tx" REAL NOT NULL DEFAULT 0,"blk_read" REAL NOT NULL DEFAULT 0,"blk_write" REAL NOT NULL DEFAULT 0,"disk_used_last" REAL NOT NULL DEFAULT 0,"disk_total_last" REAL NOT NULL DEFAULT 0,"disk_source" TEXT NOT NULL DEFAULT '',"exit_code_last" INTEGER NOT NULL DEFAULT 0,PRIMARY KEY ("kind","host_id","entity_key","tier","bucket_start")) WITHOUT ROWID;
CREATE INDEX `idx_history_host` ON `history_buckets`(`host_id`);
CREATE TABLE "runtime_stop_intents" ("container_id" TEXT NOT NULL,"container_name" TEXT NOT NULL,"project_id" TEXT NOT NULL,"recorded_at" datetime NOT NULL,PRIMARY KEY ("container_id"));
CREATE TABLE "notification_outbox_next" ("id" TEXT NOT NULL,"event" TEXT NOT NULL,"environment_id" TEXT NOT NULL,"dedup_key" TEXT NOT NULL,"payload" TEXT NOT NULL,"status" TEXT NOT NULL,"attempts" INTEGER NOT NULL DEFAULT 0,"lease_until" datetime,"created_at" datetime NOT NULL,"settled_at" datetime,PRIMARY KEY ("id"),CHECK (event IN ('deploy_failed','backup_failed','host_offline','auto_heal','sustained_load')),CHECK (status IN ('pending','sending','sent','failed','unconfigured','expired')),CHECK (attempts >= 0),CHECK ((status='sending') = (lease_until IS NOT NULL)),CHECK ("payload" IS NULL OR json_valid("payload")));
INSERT INTO notification_outbox_next (id,event,environment_id,dedup_key,payload,status,attempts,lease_until,created_at,settled_at) SELECT id,event,environment_id,dedup_key,payload,status,attempts,lease_until,created_at,settled_at FROM notification_outbox;
DROP TABLE notification_outbox;
ALTER TABLE notification_outbox_next RENAME TO notification_outbox;
CREATE UNIQUE INDEX `idx_notification_outbox_dedup_key` ON `notification_outbox`(`dedup_key`);
CREATE INDEX idx_notification_outbox_delivery ON notification_outbox (status,created_at);
ALTER TABLE users ADD COLUMN "ui_preferences" TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(ui_preferences));
