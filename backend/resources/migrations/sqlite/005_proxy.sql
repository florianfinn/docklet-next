-- Source: docklet-next/issues/473
-- Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
-- Retrieved: 2026-11-15
-- License: Apache-2.0
-- Deviations: Original engine-independent proxy contract foundation.

-- +goose Up
CREATE TABLE "proxy_generation" ("id" INTEGER PRIMARY KEY NOT NULL CHECK (id = 1),"generation" TEXT NOT NULL);
INSERT INTO proxy_generation VALUES (1, lower(hex(randomblob(16))));
CREATE TABLE "proxy_instances" ("id" TEXT NOT NULL,"environment_id" TEXT NOT NULL,"data" BLOB NOT NULL,PRIMARY KEY ("id"));
CREATE TABLE "proxy_operations" ("id" TEXT NOT NULL,"instance_id" TEXT NOT NULL,"generation" TEXT NOT NULL,"digest" TEXT NOT NULL,"registration" TEXT NOT NULL,"state" TEXT NOT NULL,"reason" TEXT NOT NULL DEFAULT '',"evidence" TEXT NOT NULL DEFAULT '',"actor" TEXT NOT NULL DEFAULT '',"request" BLOB NOT NULL,"result" BLOB NOT NULL,"created_at" datetime NOT NULL,"finished_at" datetime,PRIMARY KEY ("id"));
CREATE INDEX proxy_operations_instance ON proxy_operations(instance_id, created_at);
CREATE TABLE "proxy_reservations" ("instance_id" TEXT NOT NULL,"operation_id" TEXT NOT NULL,PRIMARY KEY ("instance_id"),UNIQUE (operation_id));
CREATE TABLE "proxy_journal" ("id" TEXT NOT NULL,"source" TEXT NOT NULL,"generation" TEXT NOT NULL,"digest" TEXT NOT NULL,"registration" TEXT NOT NULL,"state" TEXT NOT NULL,"phase" TEXT NOT NULL,"intent" BLOB NOT NULL,"result" BLOB NOT NULL,"created_at" datetime NOT NULL,"finished_at" datetime,PRIMARY KEY ("id"));
CREATE UNIQUE INDEX proxy_journal_source ON proxy_journal(source) WHERE finished_at IS NULL;
CREATE TABLE "proxy_fences" ("id" TEXT NOT NULL,"generation" TEXT NOT NULL,"registration" TEXT NOT NULL,PRIMARY KEY ("id"));
CREATE TABLE "proxy_verified_files" ("source" TEXT NOT NULL,"path" TEXT NOT NULL,"hash" TEXT NOT NULL,"content" BLOB NOT NULL,PRIMARY KEY ("source","path"));
CREATE TABLE "proxy_admissions" ("id" TEXT NOT NULL,"generation" TEXT NOT NULL,"registration" TEXT NOT NULL,PRIMARY KEY ("id"));
