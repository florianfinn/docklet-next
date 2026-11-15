// Source: docklet-next/issues/462
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original shell read with result-local host recovery and identity-owned retention.

import type { NavInventory } from "@/data/nav-inventory";
import { useQuery } from "@tanstack/react-query";
import { inventoryRecoveryInterval, inventoryStructuralSharing, INVENTORY_READ_TAG } from "@/data/inventory-recovery";
import { queryKeys } from "@/data/query-keys";
import { useHosts } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";

export function useNavInventory() {
  const adapter = useAdapter(), hosts = useHosts();
  return useQuery<NavInventory[]>({ queryKey: queryKeys.navInventory, structuralSharing: inventoryStructuralSharing,
    refetchInterval: (query) => inventoryRecoveryInterval(query.state.data, hosts.data),
    queryFn: ({ signal }) => adapter.listNavInventory(undefined, signal, INVENTORY_READ_TAG) });
}
