-- Source: docklet-next/issues/680
-- Revision: 9590d35e3340294b3d329053d5846656b5791371
-- Retrieved: 2026-11-27
-- License: Apache-2.0
-- Deviations: Original minimum image age implementation.

-- +goose Up
ALTER TABLE "image_updates" ADD COLUMN "latest_manifest" text;
CREATE TABLE "image_update_first_seen" ("repository" text NOT NULL,"platform" text NOT NULL,"manifest" text NOT NULL,"first_seen_at" datetime NOT NULL,"last_seen_at" datetime NOT NULL,PRIMARY KEY ("repository","platform","manifest"));
CREATE INDEX "idx_image_update_first_seen_last_seen_at" ON "image_update_first_seen"("last_seen_at");
