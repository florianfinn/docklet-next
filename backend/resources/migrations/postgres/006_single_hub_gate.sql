-- Source: docklet-next/docs/design/run-gate.md
-- Revision: 6d7ebd157e06203720565ef9ac1952bdd17dc8d2
-- Retrieved: 2026-10-15
-- License: Apache-2.0
-- Deviations: Original forward migration for issue #43; removes obsolete multi-hub gate fields.
-- +goose Up
ALTER TABLE activities DROP COLUMN gate_owner;
ALTER TABLE activities DROP COLUMN gate_lease_at;
ALTER TABLE activities DROP COLUMN cancel_requested_at;
