// Source: docklet-next/issues/600
// Revision: 393e645a79236449c35cb8e1a4827c83bbf45da0
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original unreadable-row count helper for host observations.

import type { InventoryObservation, UnreadableKind } from "./model";

/** Rows of these kinds the host's last reads left out because they could not be read (#600). */
export function unreadableRows(inventory: InventoryObservation | undefined, kinds: readonly UnreadableKind[]): { count: number; names: string[] } {
  const rows = kinds.flatMap((kind) => inventory?.unreadable?.[kind] ? [inventory.unreadable[kind]] : []);
  return { count: rows.reduce((sum, r) => sum + r.count, 0), names: [...new Set(rows.flatMap((r) => r.names))] };
}
