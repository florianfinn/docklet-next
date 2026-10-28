import { useActionRuns } from "@/data/live";
import { useImages, useSecurity, useTopology, useVolumes } from "@/data/queries";
import { isHostVisible } from "@/domain/hosts";
import { useHostScope, useVisibleContainers, useVisibleStacks } from "@/shared/hooks/use-host-scope";

import type { NavKey } from "./nav-items";

export type NavCounts = Partial<Record<NavKey, { value: number | "?"; partial?: boolean; tone?: "err" }>>;

/** Navigation counters within the global host filter; shared by both sidebars. */
export function useNavCounts(): NavCounts {
  const stacks = useVisibleStacks();
  const containers = useVisibleContainers();
  const security = useSecurity();
  const topology = useTopology();
  const { selection, all: hosts } = useHostScope();
  const volumes = useVolumes();
  const images = useImages();
  const critical = (security.data?.images ?? []).filter((i) => !i.unavailable && !!i.scannedAt && isHostVisible(selection, i.hostId) && i.usage === "running").reduce((n, i) => n + i.counts.critical, 0);
  const partial = !!security.data?.unavailable?.some((h) => isHostVisible(selection, h.hostId)) || !!security.data?.images.some((i) => i.unavailable && isHostVisible(selection, i.hostId));
  return {
    stacks: stacks.data ? { value: stacks.visible.length } : undefined,
    containers: containers.data ? { value: containers.visible.length } : undefined,
    security: security.data?.enabled && (critical || partial) ? { value: critical || "?", partial, tone: critical ? "err" : undefined } : undefined,
    networks: topology.data ? { value: topology.data.networks.filter((n) => isHostVisible(selection, n.hostId)).length } : undefined,
    volumes: volumes.data ? { value: volumes.data.filter((v) => isHostVisible(selection, v.hostId)).length } : undefined,
    images: images.data ? { value: images.data.filter((i) => isHostVisible(selection, i.hostId)).length } : undefined,
    // Hub-wide list: all hosts, independent of the filter.
    hosts: hosts.length ? { value: hosts.length } : undefined,
  };
}

export function useRunningActionCount(): number {
  return useActionRuns().filter((r) => r.status === "queued" || r.status === "running").length;
}

export function isNavActive(pathname: string, to: string): boolean {
  return pathname === to || pathname.startsWith(`${to}/`);
}
