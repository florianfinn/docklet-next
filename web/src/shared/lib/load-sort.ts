import type { MetricsSnapshot } from "@/domain/model";

import type { SortDir, SortState } from "./table-sort";

/** Which live value the load column sorts by: CPU percent or used memory in MB. */
export type LoadMetric = "cpu" | "mem";

/** Live value of one or more containers; null when nothing is measured (stopped, disconnected host). */
export function loadValue(snapshot: MetricsSnapshot | undefined, containerIds: readonly string[], metric: LoadMetric): number | null {
  let total = 0;
  let measured = false;
  for (const id of containerIds) {
    const m = snapshot?.containers[id];
    if (!m || m.memMb === 0) continue;
    measured = true;
    total += metric === "cpu" ? m.cpu : m.memMb;
  }
  return measured ? total : null;
}

/** Choosing a metric sorts by it; an active load sort keeps its direction, otherwise highest first. */
export function sortForMetric<K extends string>(sort: SortState<K>, loadKey: K): SortState<K> {
  return { key: loadKey, dir: sort.key === loadKey ? sort.dir : ("desc" as SortDir) };
}
