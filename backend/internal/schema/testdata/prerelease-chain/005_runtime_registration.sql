-- Source: docklet-next/docs/design/runtime-control.md
-- Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
-- Retrieved: 2026-10-20
-- License: Apache-2.0
-- Deviations: Persist the authenticated executor registration independently of history.

-- +goose Up
CREATE TABLE "runtime_executor_binding" ("id" INTEGER PRIMARY KEY NOT NULL,"manager_id" TEXT NOT NULL,"registration_id" TEXT NOT NULL,CHECK (id = 1),CHECK (length(manager_id) > 0),CHECK (length(registration_id) > 0));
