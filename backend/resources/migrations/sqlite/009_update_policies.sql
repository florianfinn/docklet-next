-- Source: docklet-next/docs/design/update-policies.md
-- Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
-- Retrieved: 2026-10-25
-- License: Apache-2.0
-- Deviations: Forward hub policy, schedule, child and candidate notification schema from canonical models/integrity.

-- +goose Up
CREATE TABLE "image_policy_children" ("id" TEXT NOT NULL,"job_run_id" TEXT NOT NULL,"policy_id" TEXT NOT NULL,"revision" INTEGER NOT NULL,"status" TEXT NOT NULL,"code" TEXT NOT NULL,"runtime_id" TEXT NOT NULL,PRIMARY KEY ("id"),CHECK (revision > 0),CHECK (status IN ('prepared','admitted','refused','settled')));
CREATE TABLE "image_update_candidates" ("id" TEXT NOT NULL,"policy_id" TEXT NOT NULL,"created_at" datetime NOT NULL,PRIMARY KEY ("id"));
CREATE TABLE "image_update_notifications" ("id" TEXT NOT NULL,"candidate_id" TEXT NOT NULL,"environment_id" TEXT NOT NULL,"payload" TEXT NOT NULL,"status" TEXT NOT NULL,"delivered" INTEGER NOT NULL,"created_at" datetime NOT NULL,PRIMARY KEY ("id"),CHECK (status IN ('pending','sending','sent','unconfigured','failed')),CHECK (delivered >= 0));
CREATE TABLE "image_update_schedules" ("id" TEXT NOT NULL,"environment_id" TEXT NOT NULL,"registration_id" TEXT NOT NULL,"revision" INTEGER NOT NULL,"polling_interval" TEXT NOT NULL,"auto_update_interval" TEXT NOT NULL,"polling_enabled" numeric NOT NULL,"auto_update_enabled" numeric NOT NULL,"actor_id" TEXT NOT NULL,"actor_key_id" TEXT NOT NULL,PRIMARY KEY ("id"),CHECK (revision > 0),CHECK (polling_enabled IN (0,1)),CHECK (auto_update_enabled IN (0,1)));
CREATE TABLE "stack_update_policies" ("id" TEXT NOT NULL,"environment_id" TEXT NOT NULL,"registration_id" TEXT NOT NULL,"target_id" TEXT NOT NULL,"target_identity" TEXT NOT NULL,"mode" TEXT NOT NULL,"revision" INTEGER NOT NULL,"actor_id" TEXT NOT NULL,"actor_key_id" TEXT NOT NULL,"checked_at" datetime,"code" TEXT NOT NULL,"apply_checked_at" datetime,"apply_code" TEXT NOT NULL,"bookkeeping_at" datetime,"bookkeeping_code" TEXT NOT NULL,"candidates" TEXT NOT NULL,PRIMARY KEY ("id"),CHECK (mode IN ('off','notify','apply')),CHECK (revision > 0));
CREATE INDEX `idx_image_update_candidates_policy_id` ON `image_update_candidates`(`policy_id`);
CREATE UNIQUE INDEX `idx_image_update_notifications_candidate_id` ON `image_update_notifications`(`candidate_id`);
CREATE UNIQUE INDEX `idx_image_update_schedules_environment_id` ON `image_update_schedules`(`environment_id`);
CREATE INDEX `idx_stack_update_policies_environment_id` ON `stack_update_policies`(`environment_id`);
CREATE UNIQUE INDEX `idx_update_child_target` ON `image_policy_children`(`job_run_id`,`policy_id`);
