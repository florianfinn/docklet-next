-- Source: docklet-next/issues/332
-- Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
-- Retrieved: 2026-11-03
-- License: Apache-2.0
-- Deviations: Original hub service widget storage. M17 integration of the service widget step.

-- +goose Up
CREATE TABLE "service_widgets" ("kind" TEXT NOT NULL CHECK (kind IN ('plex','overseerr','minecraft')),"enabled" INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0,1)),"address" TEXT NOT NULL DEFAULT '',"credential_enc" TEXT,"tls_pin" TEXT,"open_url" TEXT NOT NULL DEFAULT '',"stack_reference" TEXT,"revision" INTEGER NOT NULL CHECK (revision > 0),"updated_at" datetime NOT NULL,PRIMARY KEY ("kind"));
