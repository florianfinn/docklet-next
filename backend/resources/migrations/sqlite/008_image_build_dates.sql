-- Source: docklet-next/issues/634
-- Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
-- Retrieved: 2026-11-25
-- License: Apache-2.0
-- Deviations: Original image config build times of the running image and the update candidate.

-- +goose Up
ALTER TABLE "image_updates" ADD COLUMN "current_created" datetime;
ALTER TABLE "image_updates" ADD COLUMN "latest_created" datetime;
