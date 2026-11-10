-- Source: docklet-next/docs/design/runtime-control.md
-- Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
-- Retrieved: 2026-10-19
-- License: Apache-2.0
-- Deviations: Forward durable runtime output cursor/window, bounded remote message projection and removed-registration evidence.

-- +goose Up
ALTER TABLE runtime_receipts ADD COLUMN "output_next_seq" INTEGER NOT NULL DEFAULT 1;
ALTER TABLE activities ADD COLUMN "runtime_output" TEXT NOT NULL DEFAULT '[]';
ALTER TABLE activity_messages ADD COLUMN "runtime_remote" numeric NOT NULL DEFAULT false;
CREATE TABLE "runtime_removed_targets" ("registration_id" TEXT NOT NULL,"manager_id" TEXT NOT NULL,"environment_id" TEXT NOT NULL,"store_generation" TEXT NOT NULL,"executor_generation" TEXT NOT NULL,"removed_at" datetime NOT NULL,PRIMARY KEY ("registration_id"));
UPDATE activities SET output_truncated = true WHERE id IN (SELECT run_id FROM runtime_receipts WHERE kind = 'receipt') AND output <> '[]';
ALTER TABLE activities ADD COLUMN "runtime_step_trace" TEXT NOT NULL DEFAULT '[]';
