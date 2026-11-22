-- Source: docklet-next/issues/573
-- Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
-- Retrieved: 2026-11-22
-- License: Apache-2.0
-- Deviations: Original per-host network route and diagnostic tailnet node observation.

-- +goose Up
ALTER TABLE "environments" ADD COLUMN "network_route" TEXT CHECK (network_route IN ('lan','domain','tailnet'));
ALTER TABLE "environments" ADD COLUMN "tailnet_node_id" TEXT;
ALTER TABLE "environments" ADD COLUMN "tailnet_tagged" numeric;
ALTER TABLE "environments" ADD COLUMN "tailnet_observed_at" datetime;
