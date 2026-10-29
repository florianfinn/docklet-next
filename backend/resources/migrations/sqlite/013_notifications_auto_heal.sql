-- Source: docklet-next/issues/296
-- Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
-- Retrieved: 2026-10-29
-- License: Apache-2.0
-- Deviations: Original notification and Auto Heal foundation.

-- +goose Up
CREATE TABLE notification_outbox (
 id TEXT PRIMARY KEY NOT NULL, event TEXT NOT NULL CHECK (event IN ('deploy_failed','backup_failed','host_offline','auto_heal')),
 environment_id TEXT NOT NULL, dedup_key TEXT NOT NULL UNIQUE, payload TEXT NOT NULL CHECK (json_valid(payload)),
 status TEXT NOT NULL CHECK (status IN ('pending','sending','sent','failed','unconfigured','expired')),
 attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0), lease_until datetime,
 created_at datetime NOT NULL, settled_at datetime,
 CHECK ((status='sending') = (lease_until IS NOT NULL))
);
CREATE INDEX idx_notification_outbox_delivery ON notification_outbox(status, created_at);
CREATE TABLE host_availability_notifications (
 environment_id TEXT PRIMARY KEY NOT NULL, offline_since datetime NOT NULL, notified_offline_at datetime
);
CREATE TABLE auto_heal_restarts (
 id INTEGER PRIMARY KEY, run_id TEXT NOT NULL, container_id TEXT NOT NULL, container_name TEXT NOT NULL,
 stack TEXT NOT NULL, started_at datetime NOT NULL, baseline_started_at TEXT NOT NULL,
 baseline_restart_count INTEGER NOT NULL CHECK (baseline_restart_count >= 0),
 outcome TEXT NOT NULL CHECK (outcome IN ('attempting','restarted','restart-failed','interrupted','limit-reached')),
 settled_at datetime, projected_at datetime,
 CHECK ((outcome='attempting') = (settled_at IS NULL)), UNIQUE(run_id, container_id)
);
CREATE INDEX idx_auto_heal_container_started ON auto_heal_restarts(container_id, started_at);
