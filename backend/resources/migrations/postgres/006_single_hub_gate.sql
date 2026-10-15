-- Source: docklet-next/docs/design/run-gate.md
-- Revision: ab701984e86075694baabdc5efe14bed287768fc
-- Retrieved: 2026-10-14
-- License: Apache-2.0
-- Deviations: Original forward migration for issue #43; removes obsolete multi-hub gate fields.
-- +goose Up
ALTER TABLE activities DROP COLUMN gate_owner;
ALTER TABLE activities DROP COLUMN gate_lease_at;
ALTER TABLE activities DROP COLUMN cancel_requested_at;
