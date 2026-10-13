import type { ContainerInfo, ContainerOrigin, Host, MetricsSnapshot } from "@/domain/model";
import { containerRuntime, type RuntimeStatus } from "@/domain/status";
import { hasUpdate, versionLabel } from "@/domain/updates";
import { loadValue, type LoadMetric } from "@/shared/lib/load-sort";
import { sortRows, type SortDir, type SortState } from "@/shared/lib/table-sort";

export type OriginFilter = "all" | ContainerOrigin;
export type ContainerSortKey = "name" | "status" | "host" | "origin" | "version" | "update" | "load" | "restart" | "uptime";
export type ContainerSort = SortState<ContainerSortKey>;

/** Default order: host A–Z, names A–Z within each host. */
export const DEFAULT_CONTAINER_SORT: ContainerSort = { key: "host", dir: "asc" };

export const containerSortDefaultDir = (key: ContainerSortKey): SortDir => (key === "update" || key === "uptime" || key === "load" ? "desc" : "asc");

const RANK: Record<RuntimeStatus, number> = { unhealthy: 0, restarting: 1, unknown: 2, starting: 3, running: 4, stopped: 5 };
const ORIGIN_RANK: Record<ContainerOrigin, number> = { compose: 0, standalone: 1, unraid: 2, unknown: 3 };

const ORIGINS: readonly ContainerOrigin[] = ["compose", "standalone", "unraid", "unknown"];

/** Origins present among the given containers (host scope, not search), in a fixed order. */
export function availableOrigins(containers: readonly ContainerInfo[]): ContainerOrigin[] {
  const present = new Set(containers.map((c) => c.origin));
  return ORIGINS.filter((o) => present.has(o));
}

export function filterContainers(containers: ContainerInfo[], f: { query: string; origin: OriginFilter }): ContainerInfo[] {
  const q = f.query.trim().toLowerCase();
  return containers.filter((c) => (f.origin === "all" || c.origin === f.origin) && (!q || `${c.name} ${c.image.repository} ${c.stackId ?? ""}`.toLowerCase().includes(q)));
}

/**
 * Column sort with the shared tie-breakers (host, then name, then id). Load
 * uses the live CPU percent or used memory; stopped containers and those on a
 * disconnected host have no value and come last in both directions.
 */
export function sortContainers(containers: ContainerInfo[], hosts: Map<string, Host>, sort: ContainerSort, load?: { snapshot: MetricsSnapshot | undefined; metric: LoadMetric }): ContainerInfo[] {
  return sortRows(
    containers,
    sort,
    (c, key) => {
      switch (key) {
        case "name":
          return c.name;
        case "status":
          return RANK[containerRuntime(c, hosts.get(c.hostId))];
        case "host":
          return hosts.get(c.hostId)?.name ?? c.hostId;
        case "origin":
          return ORIGIN_RANK[c.origin];
        case "version":
          return versionLabel(c.image);
        case "update":
          return hasUpdate(c.update) ? 1 : 0;
        case "load":
          return hosts.get(c.hostId)?.status === "online" && c.state === "running" ? loadValue(load?.snapshot, [c.id], load?.metric ?? "cpu") : null;
        case "restart":
          return c.restartPolicy;
        case "uptime":
          return c.state === "running" && c.startedAt ? -new Date(c.startedAt).getTime() : -Infinity;
      }
    },
    (c) => ({ id: c.id, name: c.name, hostName: hosts.get(c.hostId)?.name ?? c.hostId }),
  );
}
