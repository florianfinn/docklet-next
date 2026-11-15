// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/app/shell/use-nav-counts.ts
// Revision: 7bf104baca9468a679cbd0ee87b0a47e6b2282adab3a59519a4ac180e007be44
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Historical L4 snapshot; #38 HTTP adapter, live channel, capabilities, run states and fixed history. See web/PROVENANCE.md. M11 #262: live vulnerability reads, scan controls and security settings. M20 #360: web hardening step (#359 #24 #175 #177 #383 #421 #271). M26 #462: bounded shell counts and raw inventory ownership.

import { useActionRuns } from "@/data/live";
import { useImages, useActiveSecurity, useNetworkSummaries } from "@/data/queries";
import { isHostVisible } from "@/domain/hosts";
import { useHostScope, useVisibleContainers, useVisibleStacks } from "@/shared/hooks/use-host-scope";

import { navSecurityCount } from "./nav-count-values";
import { useNavInventory } from "./use-nav-inventory";
import type { NavKey } from "./nav-items";

export type NavCounts = Partial<Record<NavKey, { value: number | "?"; partial?: boolean; tone?: "err" }>>;

/** Navigation counters within the global host filter; shared by both sidebars. */
export function useNavCounts(): NavCounts {
  const stacks = useVisibleStacks();
  const containers = useVisibleContainers();
  const security = useActiveSecurity();
  const networks = useNetworkSummaries();
  const { selection, all: hosts } = useHostScope();
  const inventory = useNavInventory();
  const images = useImages();
  const { critical, partial } = navSecurityCount(security.data, inventory.data, selection);
  return {
    stacks: stacks.data ? { value: stacks.visible.length } : undefined,
    containers: containers.data ? { value: containers.visible.length } : undefined,
    security: security.data?.enabled && (critical || partial) ? { value: critical || "?", partial, tone: critical ? "err" : undefined } : undefined,
    networks: networks.data ? { value: networks.data.filter((n) => isHostVisible(selection, n.hostId)).length } : undefined,
    volumes: inventory.data ? { value: inventory.data.filter((h) => isHostVisible(selection, h.hostId)).reduce((n, h) => n + (h.volumeTotal ?? 0), 0) } : undefined,
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
