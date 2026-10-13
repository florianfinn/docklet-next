import { useMemo } from "react";

import { useContainers, useHosts, useStacks } from "@/data/queries";
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

export function useVisibleStacks() {
  const stacks = useStacks();
  const selection = useHostSelection();
  const visible = useMemo(() => filterByHosts(stacks.data ?? [], selection), [stacks.data, selection]);
  return { ...stacks, visible };
}

export function useVisibleContainers() {
  const containers = useContainers();
  const selection = useHostSelection();
  const visible = useMemo(() => filterByHosts(containers.data ?? [], selection), [containers.data, selection]);
  return { ...containers, visible };
}
