-- Source: docklet-next/docs/design/runtime-control.md
-- Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
-- Retrieved: 2026-11-20
-- License: Apache-2.0
-- Deviations: Original peer identity confirmation contract for issue 531.

-- +goose Up
ALTER TABLE "runtime_receipts" ADD COLUMN "acknowledged_uncertainty" TEXT NOT NULL DEFAULT '';
ALTER TABLE "runtime_receipts" ADD COLUMN "peer_overrides" TEXT NOT NULL DEFAULT '{}';
