-- Source: docklet-next/docs/design/run-gate.md
-- Revision: 6d7ebd157e06203720565ef9ac1952bdd17dc8d2
-- Retrieved: 2026-10-15
-- License: Apache-2.0
-- Deviations: Original forward migration for issue #37; protects recent sync receipts during retention.
-- +goose Up
ALTER TABLE activity_sync_receipts ADD COLUMN synced_at timestamp with time zone;
UPDATE activity_sync_receipts SET synced_at = NOW();
