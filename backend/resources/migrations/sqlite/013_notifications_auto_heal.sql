-- Source: docklet-next/issues/296
-- Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
-- Retrieved: 2026-10-29
-- License: Apache-2.0
-- Deviations: Original notification and Auto Heal foundation.

-- +goose Up
CREATE TABLE "auto_heal_restarts" ("id" INTEGER PRIMARY KEY AUTOINCREMENT,"run_id" TEXT NOT NULL,"container_id" TEXT NOT NULL,"container_name" TEXT NOT NULL,"stack" TEXT NOT NULL,"started_at" datetime NOT NULL,"baseline_started_at" TEXT NOT NULL,"baseline_restart_count" INTEGER NOT NULL,"outcome" TEXT NOT NULL,"settled_at" datetime,"projected_at" datetime,CHECK (baseline_restart_count >= 0),CHECK (outcome IN ('attempting','restarted','restart-failed','interrupted','limit-reached')),CHECK ((outcome='attempting') = (settled_at IS NULL)));
CREATE TABLE "host_availability_notifications" ("environment_id" TEXT NOT NULL,"offline_since" datetime NOT NULL,"notified_offline_at" datetime,PRIMARY KEY ("environment_id"));
CREATE TABLE "notification_outbox" ("id" TEXT NOT NULL,"event" TEXT NOT NULL,"environment_id" TEXT NOT NULL,"dedup_key" TEXT NOT NULL,"payload" TEXT NOT NULL,"status" TEXT NOT NULL,"attempts" INTEGER NOT NULL DEFAULT 0,"lease_until" datetime,"created_at" datetime NOT NULL,"settled_at" datetime,PRIMARY KEY ("id"),CHECK (event IN ('deploy_failed','backup_failed','host_offline','auto_heal')),CHECK (status IN ('pending','sending','sent','failed','unconfigured','expired')),CHECK (attempts >= 0),CHECK ((status='sending') = (lease_until IS NOT NULL)),CHECK ("payload" IS NULL OR json_valid("payload")));
CREATE INDEX `idx_auto_heal_container_started` ON `auto_heal_restarts`(`container_id`,`started_at`);
CREATE UNIQUE INDEX `idx_auto_heal_run_container` ON `auto_heal_restarts`(`run_id`,`container_id`);
CREATE UNIQUE INDEX `idx_notification_outbox_dedup_key` ON `notification_outbox`(`dedup_key`);
CREATE INDEX idx_notification_outbox_delivery ON notification_outbox (status,created_at);
