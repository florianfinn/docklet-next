import type { Host, MetricsSnapshot, Stack } from "@/domain/model";
import { stackStatus, type StackStatus } from "@/domain/status";
import { summarizeUpdates } from "@/domain/updates";
import { loadValue, type LoadMetric } from "@/shared/lib/load-sort";
import { sortRows, type SortDir, type SortState } from "@/shared/lib/table-sort";

export type StatusFilter = "all" | "running" | "problem" | "updates" | "stopped";
export type StackSortKey = "name" | "status" | "host" | "services" | "updates" | "load" | "auto" | "deploy";
export type StackSort = SortState<StackSortKey>;

/** Default order: host A–Z, names A–Z within each host. */
export const DEFAULT_STACK_SORT: StackSort = { key: "host", dir: "asc" };

/** Counts and dates start with the largest or newest first. */
export const stackSortDefaultDir = (key: StackSortKey): SortDir => (key === "updates" || key === "deploy" || key === "services" || key === "load" ? "desc" : "asc");

/** Container ids of a stack's services in the metrics snapshot. */
export function stackContainerIds(stack: Stack): string[] {
  return stack.services.map((s) => `c-${stack.id}-${s.name}`);
}

/** Live values for the load sort; without a snapshot every stack counts as unmeasured. */
export interface LoadInput {
  snapshot: MetricsSnapshot | undefined;
  metric: LoadMetric;
}

export interface StackFilterInput {
  query: string;
  status: StatusFilter;
  labels: string[];
}

const STATUS_RANK: Record<StackStatus, number> = { problem: 0, unreachable: 1, busy: 2, partial: 3, running: 4, stopped: 5 };

export function filterStacks(stacks: Stack[], hosts: Map<string, Host>, f: StackFilterInput): Stack[] {
  const q = f.query.trim().toLowerCase();
  return stacks.filter((s) => {
    if (q) {
      const hay = `${s.name} ${s.description ?? ""} ${s.labels.join(" ")} ${s.services.map((x) => `${x.name} ${x.image.repository}`).join(" ")}`.toLowerCase();
      if (!hay.includes(q)) return false;
    }
    if (f.labels.length && !f.labels.every((l) => s.labels.includes(l))) return false;
    const st = stackStatus(s, hosts.get(s.hostId));
    switch (f.status) {
      case "running":
        return st === "running";
      case "problem":
        return st === "problem" || st === "unreachable";
      case "updates":
        return summarizeUpdates(s.services).pending > 0;
      case "stopped":
        return st === "stopped";
      default:
        return true;
    }
  });
}

/**
 * Column sort with the shared tie-breakers (host, then name, then id). Load
 * sums the live values of the running services; stacks without any measured
 * service (stopped, host disconnected) come last in both directions.
 */
export function sortStacks(stacks: Stack[], hosts: Map<string, Host>, sort: StackSort, load?: LoadInput): Stack[] {
  return sortRows(
    stacks,
    sort,
    (s, key) => {
      switch (key) {
        case "name":
          return s.name;
        case "status":
          return STATUS_RANK[stackStatus(s, hosts.get(s.hostId))];
        case "host":
          return hosts.get(s.hostId)?.name ?? s.hostId;
        case "services":
          return s.serviceCount ?? s.services.length;
        case "updates":
          return summarizeUpdates(s.services).pending;
        case "load":
          return hosts.get(s.hostId)?.status === "online" ? loadValue(load?.snapshot, stackContainerIds(s), load?.metric ?? "cpu") : null;
        case "auto":
          return s.autoUpdate.mode === "scheduled" ? 1 : 0;
        case "deploy":
          return s.lastDeployAt ? new Date(s.lastDeployAt).getTime() : 0;
      }
    },
    (s) => ({ id: s.id, name: s.name, hostName: hosts.get(s.hostId)?.name ?? s.hostId }),
  );
}

export function allLabels(stacks: Stack[]): string[] {
  return [...new Set(stacks.flatMap((s) => s.labels))].sort((a, b) => a.localeCompare(b, "de"));
}
