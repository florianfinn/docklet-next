// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation; combined update codes (#682).

import type { components } from "@/api/generated/schema";
import type { CatalogueKey } from "./wire-catalogue";
import { IMAGE_ERROR_KEYS, HOST_CODE_KEYS } from "./wire-catalogue-errors";

type Schema = components["schemas"];

export const SELF_UPDATE_CODE_KEYS = {
  "feature-disabled": IMAGE_ERROR_KEYS["feature-disabled"],
  "update.check-unavailable": IMAGE_ERROR_KEYS["update.check-unavailable"],
  "host.busy": HOST_CODE_KEYS["host.busy"],
  "update.self-not-compose": "images.errors.update_self-not-compose",
  "update.self-unsupported-layout": "images.errors.update_self-unsupported-layout",
  "update.self-custom-image": "images.errors.update_self-custom-image",
  "update.requires-hub-update": IMAGE_ERROR_KEYS["update.requires-hub-update"],
  "update.self-failed": "images.errors.update_self-failed",
  "update.outcome-unknown": "images.errors.update_outcome-unknown",
  "update.invalid-target": IMAGE_ERROR_KEYS["update.invalid-target"],
  "update.system-version-unknown": IMAGE_ERROR_KEYS["update.system-version-unknown"],
  "agent.unavailable": IMAGE_ERROR_KEYS["agent.unavailable"],
} satisfies Record<NonNullable<Schema["SystemSelfUpdateEligibility"]["code"] | Schema["SystemSelfUpdateResponse"]["code"]>, CatalogueKey>;

export const SELF_UPDATE_STATE_KEYS = {
  "available": "runMessages.selfUpdate.available",
  "current": "runMessages.selfUpdate.current",
  "checking": "runMessages.selfUpdate.checking",
  "not-eligible": "runMessages.selfUpdate.not-eligible",
  "unavailable": "runMessages.selfUpdate.unavailable",
} satisfies Record<NonNullable<Schema["SystemSelfUpdateEligibility"]["state"]>, CatalogueKey>;

export const SELF_UPDATE_PHASE_KEYS = {
  "reserved": "runMessages.selfUpdate.reserved",
  "helper-started": "runMessages.selfUpdate.helper-started",
  "settled": "runMessages.selfUpdate.settled",
} satisfies Record<NonNullable<Schema["SystemSelfUpdateOperation"]["phase"]>, CatalogueKey>;

export const SELF_UPDATE_OUTCOME_KEYS = {
  "healthy": "runMessages.selfUpdate.healthy",
  "failed-health": "runMessages.selfUpdate.failed-health",
  "failed-pre-effect": "runMessages.selfUpdate.failed-pre-effect",
  "unknown": "runMessages.selfUpdate.unknown",
} satisfies Record<NonNullable<Schema["SystemSelfUpdateOperation"]["outcome"]>, CatalogueKey>;

// Codes of the combined update's targets, refusals and child runs.
export const FLEET_CODE_KEYS = {
  ...SELF_UPDATE_CODE_KEYS,
  "update.all-busy": "images.errors.update_all-busy",
  "update.current": "images.errors.update_current",
  "update.hub-failed": "images.errors.update_hub-failed",
  "update.newer-than-target": "images.errors.update_newer-than-target",
  "update.fleet-unreadable": "images.errors.update_fleet-unreadable",
  "update.permission-denied": IMAGE_ERROR_KEYS["update.permission-denied"],
  "update.target-changed": IMAGE_ERROR_KEYS["update.target-changed"],
  "agent.update-required": "common.commandErrors.updateRequired",
} satisfies Record<NonNullable<Schema["SystemFleetUpdateTarget"]["code"]>, CatalogueKey>;

export const FLEET_PHASE_KEYS = {
  hub: "runMessages.fleet.hub", agents: "runMessages.fleet.agents", settled: "runMessages.fleet.settled",
} satisfies Record<NonNullable<Schema["SystemFleetUpdateStatus"]["phase"]>, CatalogueKey>;
export const FLEET_OUTCOME_KEYS = {
  success: "runMessages.fleet.success", partial: "runMessages.fleet.partial", failed: "runMessages.fleet.failed",
} satisfies Record<NonNullable<Schema["SystemFleetUpdateStatus"]["outcome"]>, CatalogueKey>;
export const FLEET_STATE_KEYS = {
  pending: "runMessages.fleet.pending", reserved: "runMessages.fleet.reserved", skipped: "runMessages.fleet.skipped", settled: "runMessages.fleet.settled",
} satisfies Record<Schema["SystemFleetUpdateTarget"]["state"], CatalogueKey>;
export const FLEET_TARGET_OUTCOME_KEYS = {
  healthy: "runMessages.selfUpdate.healthy", current: "runMessages.selfUpdate.current",
  "failed-health": "runMessages.selfUpdate.failed-health", "failed-pre-effect": "runMessages.selfUpdate.failed-pre-effect",
  unknown: "runMessages.selfUpdate.unknown", skipped: "runMessages.fleet.skipped",
} satisfies Record<NonNullable<Schema["SystemFleetUpdateTarget"]["outcome"]>, CatalogueKey>;
