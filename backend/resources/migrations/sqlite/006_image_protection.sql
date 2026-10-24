-- Source: docklet-next/docs/design/images-updates.md
-- Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
-- Retrieved: 2026-10-24
-- License: Apache-2.0
-- Deviations: Original persisted image protections for issue #229.

-- +goose Up
CREATE TABLE "image_protections" ("image_id" TEXT NOT NULL,"reason" TEXT NOT NULL,"owner_id" TEXT NOT NULL,PRIMARY KEY ("image_id","reason","owner_id"),CHECK (reason IN ('system', 'restore-point', 'active-update')),CHECK (length(image_id) > 0),CHECK (length(owner_id) > 0));
ALTER TABLE image_updates ADD COLUMN "platform" TEXT;
