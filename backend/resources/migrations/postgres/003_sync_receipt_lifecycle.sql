-- Source: florianfinn/docklet-next/backend/resources/migrations/postgres/002_run_traces.sql
-- Revision: 9dc69ac97e2670657c7bf8b6a0b05125b31cab2a
-- Retrieved: 2026-10-14
-- License: Apache-2.0
-- Deviations: Record synchronization time to protect receipts during source retirement.
-- +goose Up
ALTER TABLE activity_sync_receipts ADD COLUMN synced_at timestamp with time zone;
UPDATE activity_sync_receipts SET synced_at = NOW();
