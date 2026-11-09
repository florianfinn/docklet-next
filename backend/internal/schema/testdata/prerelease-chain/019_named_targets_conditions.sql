-- Source: docklet-next/issues/351
-- Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
-- Retrieved: 2026-11-06
-- License: Apache-2.0
-- Deviations: Original named targets and durable unhealthy-condition foundation.

-- +goose Up
ALTER TABLE notification_settings ADD COLUMN "name" TEXT NOT NULL DEFAULT '';
WITH numbered AS (
  SELECT id, CASE provider WHEN 'email' THEN 'Email' WHEN 'discord' THEN 'Discord' WHEN 'telegram' THEN 'Telegram' WHEN 'signal' THEN 'Signal' WHEN 'slack' THEN 'Slack' WHEN 'ntfy' THEN 'ntfy' WHEN 'pushover' THEN 'Pushover' WHEN 'gotify' THEN 'Gotify' WHEN 'matrix' THEN 'Matrix' WHEN 'googlechat' THEN 'Google Chat' WHEN 'generic' THEN 'Generic webhook' ELSE provider END AS label,
         ROW_NUMBER() OVER (PARTITION BY provider ORDER BY created_at, id) AS number
  FROM notification_settings
)
UPDATE notification_settings SET name = (
  SELECT label || CASE WHEN number = 1 THEN '' ELSE ' ' || number END
  FROM numbered WHERE numbered.id = notification_settings.id
);
CREATE UNIQUE INDEX notification_settings_name_unique ON notification_settings(name COLLATE NOCASE);
CREATE TABLE "history_conditions" ("host_id" TEXT NOT NULL,"kind" TEXT NOT NULL,"entity_id" TEXT NOT NULL,"revision" TEXT NOT NULL,"condition" TEXT NOT NULL,"restarts" TEXT NOT NULL DEFAULT '[]',"unhealthy_since" INTEGER,"unhealthy_notified_at" INTEGER,PRIMARY KEY ("host_id","kind","entity_id"),CHECK (kind IN ('container','stack'))) WITHOUT ROWID;
CREATE TABLE "notification_outbox_next" ("id" TEXT NOT NULL,"event" TEXT NOT NULL,"environment_id" TEXT NOT NULL,"dedup_key" TEXT NOT NULL,"payload" TEXT NOT NULL,"status" TEXT NOT NULL,"attempts" INTEGER NOT NULL DEFAULT 0,"lease_until" datetime,"created_at" datetime NOT NULL,"settled_at" datetime,PRIMARY KEY ("id"),CHECK (event IN ('deploy_failed','backup_failed','host_offline','auto_heal','sustained_load','container_unhealthy')),CHECK (status IN ('pending','sending','sent','failed','unconfigured','expired')),CHECK (attempts >= 0),CHECK ((status='sending') = (lease_until IS NOT NULL)),CHECK ("payload" IS NULL OR json_valid("payload")));
INSERT INTO notification_outbox_next (id,event,environment_id,dedup_key,payload,status,attempts,lease_until,created_at,settled_at) SELECT id,event,environment_id,dedup_key,payload,status,attempts,lease_until,created_at,settled_at FROM notification_outbox;
DROP TABLE notification_outbox;
ALTER TABLE notification_outbox_next RENAME TO notification_outbox;
CREATE UNIQUE INDEX `idx_notification_outbox_dedup_key` ON `notification_outbox`(`dedup_key`);
CREATE INDEX idx_notification_outbox_delivery ON notification_outbox (status,created_at);
