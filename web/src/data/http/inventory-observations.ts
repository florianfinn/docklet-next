// Source: docklet-next/issues/428
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original identity-owned inventory scope retention and automatic read lifetimes. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags.

import type { InventoryObservation } from "@/domain/model";

export const transientInventoryReasons = new Set(["http.error", "inventory.read-error", "agent.unavailable", "vulnerability.read-timeout", "vulnerability.read-unavailable", "vulnerability.storage-unavailable", "capability.read-unavailable"]);

/** Visible refusals and recovery are independent facts; neither depends on merge order. */
export function mergeInventoryObservations(observations: readonly InventoryObservation[]): InventoryObservation {
  const reasons = observations.flatMap((o) => o.reason ? [o.reason] : []);
  const reason = reasons.find((code) => !transientInventoryReasons.has(code)) ?? reasons[0];
  const recovery = observations.flatMap((o) => o.recovery ? [o.recovery] : o.reason && transientInventoryReasons.has(o.reason) ? [o.reason] : [])[0];
  return { observedAt: observations.flatMap((o) => o.observedAt ? [o.observedAt] : []).sort()[0], stale: observations.some((o) => o.stale), ...(reason ? { reason } : {}), ...(recovery ? { recovery } : {}), ...(observations.some((o) => o.scopeStale) ? { scopeStale: true } : {}) };
}
