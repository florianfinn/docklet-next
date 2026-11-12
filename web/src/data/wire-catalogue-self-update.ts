// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

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
