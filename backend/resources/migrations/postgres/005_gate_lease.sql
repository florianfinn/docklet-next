-- Source: docklet-next/docs/design/api-contract.md
-- Revision: 1d6b25c3162636f018e0a6491e7f5428110874e3
-- Retrieved: 2026-10-14
-- License: Apache-2.0
-- Deviations: Original forward migration for issue #37; adds lease and cancellation fields.
-- +goose Up
ALTER TABLE activities ADD COLUMN gate_lease_at timestamptz;
ALTER TABLE activities ADD COLUMN cancel_requested_at timestamptz;
UPDATE activities SET gate_lease_at = NOW()
WHERE COALESCE(gate_scope, '') <> '' AND status IN ('queued', 'running');
