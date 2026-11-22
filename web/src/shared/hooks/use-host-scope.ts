import { useMemo } from "react";

import { useContainers, useHosts, useStacks } from "@/data/queries";
import { inventoryHostsPending } from "@/data/inventory-recovery";
import { filterByHosts, isHostVisible, sortHosts } from "@/domain/hosts";
import type { Host } from "@/domain/model";
import { useHostSelection } from "@/shared/preferences/preferences-store";

const NO_HOSTS: Host[] = [];

/** All hosts sorted by name, the visible subset of the sidebar filter and a lookup map. */
export function useHostScope() {
  const hosts = useHosts();
  const selection = useHostSelection();
  const all = useMemo(() => (hosts.data ? sortHosts(hosts.data) : NO_HOSTS), [hosts.data]);
  const visible = useMemo(() => all.filter((h) => isHostVisible(selection, h.id)), [all, selection]);
  const byId = useMemo(() => new Map<string, Host>(all.map((h) => [h.id, h])), [all]);
  return { ...hosts, all, visible, byId, selection, filtered: selection.length > 0 };
}

/** True while a host of the current scope (selected hosts, or all) has not answered the inventory read yet. */
export function useHostsPending(data: unknown): boolean {
  const hosts = useHosts();
  const selection = useHostSelection();
  return useMemo(() => {
    const known = (hosts.data ?? []).map((h) => h.id);
    const scope = selection.length ? known.filter((id) => selection.includes(id)) : known;
    return inventoryHostsPending(data, scope);
  }, [data, hosts.data, selection]);
}

export function useVisibleStacks() {
  const stacks = useStacks();
  const selection = useHostSelection();
  const visible = useMemo(() => filterByHosts(stacks.data ?? [], selection), [stacks.data, selection]);
  const hostsPending = useHostsPending(stacks.data);
  return { ...stacks, visible, hostsPending };
}

export function useVisibleContainers() {
  const containers = useContainers();
  const selection = useHostSelection();
  const visible = useMemo(() => filterByHosts(containers.data ?? [], selection), [containers.data, selection]);
  const hostsPending = useHostsPending(containers.data);
  return { ...containers, visible, hostsPending };
}
