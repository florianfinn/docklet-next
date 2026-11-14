// Source: docklet-next/issues/462
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original internal shell inventory projection; no wire contract changes.

import type { ContainerInfo, InventoryObservation } from "@/domain/model";
import { withoutSystem } from "@/domain/system";

/** Raw daemon bind sources stay separate from display mount specifications. */
export interface InventoryContainer extends ContainerInfo { bindSources: string[] }
export interface NavInventory {
  hostId: string;
  containers: InventoryContainer[];
  inventoryUnavailable: boolean;
  volumeTotal?: number;
  inventory?: InventoryObservation;
}

export function distinctBindCount(containers: readonly InventoryContainer[]): number {
  return new Set(withoutSystem(containers).flatMap((c) => c.bindSources)).size;
}
