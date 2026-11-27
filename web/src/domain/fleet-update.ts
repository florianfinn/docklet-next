// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original combined update web control and progress.

import type { components } from "@/api/generated/schema";
import { FLEET_CODE_KEYS, FLEET_PHASE_KEYS, FLEET_OUTCOME_KEYS, FLEET_STATE_KEYS, FLEET_TARGET_OUTCOME_KEYS } from "@/data/wire-catalogue-self-update";
import { wireKey } from "@/data/wire-catalogue";

export type FleetStatus = components["schemas"]["SystemFleetUpdateStatus"];
export type FleetResponse = components["schemas"]["SystemFleetUpdateResponse"];
export type FleetTarget = components["schemas"]["SystemFleetUpdateTarget"];
const text = (v: unknown): v is string => typeof v === "string" && !!v.trim();
export function validFleetTarget(v: FleetTarget): boolean {
  return !!v && text(v.environmentId) && text(v.opId) && !!wireKey(FLEET_STATE_KEYS, v.state)
    && (v.outcome === undefined || !!wireKey(FLEET_TARGET_OUTCOME_KEYS, v.outcome))
    && (v.code === undefined || !!wireKey(FLEET_CODE_KEYS, v.code))
    && (v.runId === undefined || text(v.runId));
}
export function validFleetStatus(v: FleetStatus): boolean {
  if (!v || typeof v.active !== "boolean" || !(v.agents === null || Array.isArray(v.agents) && v.agents.every(validFleetTarget))) return false;
  if (v.hub !== undefined && !validFleetTarget(v.hub)) return false;
  if (v.phase !== undefined && !wireKey(FLEET_PHASE_KEYS, v.phase) || v.outcome !== undefined && !wireKey(FLEET_OUTCOME_KEYS, v.outcome)) return false;
  if (![v.fleetId, v.runId, v.targetVersion].every(x => x === undefined || text(x))) return false;
  if (v.active || v.fleetId !== undefined) {
    if (![v.fleetId, v.runId, v.targetVersion].every(text) || !v.hub || !v.phase) return false;
    if (v.active ? v.phase === "settled" || v.outcome !== undefined : v.phase !== "settled" || !v.outcome) return false;
  }
  const ids = [v.hub, ...(v.agents ?? [])].filter(Boolean).map(x => x!.environmentId);
  return new Set(ids).size === ids.length;
}
export function validFleetResponse(v: FleetResponse): boolean {
  return !!v && text(v.fleetId) && text(v.runId) && (v.hubPhase === "current" || v.hubPhase === "update" && text(v.hubOpId));
}
