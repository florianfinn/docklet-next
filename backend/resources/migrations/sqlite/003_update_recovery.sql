-- Source: docklet-next/issues/440
-- Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
-- Retrieved: 2026-11-12
-- License: Apache-2.0
-- Deviations: Original durable observed-run and failed-candidate recovery storage.

-- +goose Up
CREATE TABLE "runtime_observed_runs" ("run_id" TEXT NOT NULL,"kind" TEXT NOT NULL,"mode" TEXT NOT NULL,"evidence" TEXT NOT NULL DEFAULT '',"outcome" TEXT NOT NULL DEFAULT '',"created_at" datetime NOT NULL,"settled_at" datetime,PRIMARY KEY ("run_id"),CHECK (kind IN ('stack','container')),CHECK (mode IN ('scheduled','manual')),CHECK (length(CAST(evidence AS BLOB)) <= 65536),CHECK (outcome IN ('','restored','restore-failed','unavailable','not-needed','interrupted')));
CREATE TABLE "image_policy_failed_candidates" ("policy_id" TEXT NOT NULL,"target_identity" TEXT NOT NULL,"service" TEXT NOT NULL,"repository" TEXT NOT NULL,"platform" TEXT NOT NULL,"manifest" TEXT NOT NULL,"run_id" TEXT NOT NULL,"created_at" datetime NOT NULL,PRIMARY KEY ("policy_id","target_identity","service","repository","platform","manifest"));
