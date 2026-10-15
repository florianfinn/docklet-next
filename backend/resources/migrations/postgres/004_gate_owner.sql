-- Source: docklet-next/docs/design/api-contract.md
-- Revision: 1d6b25c3162636f018e0a6491e7f5428110874e3
-- Retrieved: 2026-10-14
-- License: Apache-2.0
-- Deviations: Original forward migration for issue #37; adds gated-run owner fencing.
-- +goose Up
ALTER TABLE activities ADD COLUMN gate_owner text;
