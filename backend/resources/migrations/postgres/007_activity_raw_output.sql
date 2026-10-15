-- Source: docklet-next/docs/design/adapter-live-capabilities.md
-- Revision: dfdfa084478d327818d8c57b0973d638f80861f6
-- Retrieved: 2026-10-15
-- License: Apache-2.0
-- Deviations: Original implementation for issue #38.
-- +goose Up
ALTER TABLE activities ADD COLUMN output jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE activities ADD COLUMN output_truncated boolean NOT NULL DEFAULT false;
