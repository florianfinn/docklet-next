// Source: docklet-next/issues/462
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original read-only shell security count projection from unfiltered running containers.

import type { ActiveSecurity } from "@/domain/model";
import type { NavInventory } from "@/data/nav-inventory";
import { isHostVisible } from "@/domain/hosts";

export function navSecurityCount(security: ActiveSecurity | undefined, inventory: readonly NavInventory[] | undefined, selection: readonly string[]) {
  const running = new Set((inventory ?? []).filter((h) => !h.inventoryUnavailable).flatMap((h) => h.containers.filter((c) => c.state === "running").map((c) => c.imageId)));
  const critical = (security?.images ?? []).filter((i) => !i.unavailable && !!i.scannedAt && isHostVisible(selection, i.hostId) && running.has(`${i.hostId}~${i.id}`)).reduce((n, i) => n + i.counts.critical, 0);
  const partial = !!inventory?.some((h) => h.inventoryUnavailable && isHostVisible(selection, h.hostId)) || !!security?.unavailable.some((h) => isHostVisible(selection, h.hostId)) || !!security?.images.some((i) => i.unavailable && isHostVisible(selection, i.hostId));
  return { critical, partial };
}
