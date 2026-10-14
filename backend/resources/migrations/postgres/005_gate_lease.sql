-- Source: florianfinn/docklet-next/backend/resources/migrations/postgres/004_gate_owner.sql
-- Revision: 660e9e31bfeb0c724d61ee5d43aec3288c84289d
-- Retrieved: 2026-10-14
-- License: Apache-2.0
-- Deviations: Separate gated-run lease liveness and durable cancellation request.
-- +goose Up
ALTER TABLE activities ADD COLUMN gate_lease_at timestamptz;
ALTER TABLE activities ADD COLUMN cancel_requested_at timestamptz;
UPDATE activities SET gate_lease_at = NOW()
WHERE COALESCE(gate_scope, '') <> '' AND status IN ('queued', 'running');
