-- Source: florianfinn/docklet-next/backend/resources/migrations/postgres/003_sync_receipt_lifecycle.sql
-- Revision: d4e7845e461f51d56a63d3a43937679d6449cf3c
-- Retrieved: 2026-10-14
-- License: Apache-2.0
-- Deviations: Fence gated workers after lease loss.
-- +goose Up
ALTER TABLE activities ADD COLUMN gate_owner text;
