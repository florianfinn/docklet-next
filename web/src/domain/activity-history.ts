import type { ActionRun, OpsEvent } from "./model";

/**
 * Size of the activity history kept by the hub, as entry counts rather than
 * an age: 500 events cover several weeks of a small fleet (about a dozen per
 * day, more during incidents); 100 finished actions cover the recent deploys,
 * updates, backups and scans with their output. Running actions never count
 * and are never removed.
 */
export const HISTORY_LIMITS = {
  events: { min: 50, max: 5000, default: 500 },
  actions: { min: 10, max: 1000, default: 100 },
} as const;

export type HistoryLimitError = "range" | "integer";

export function validateHistoryLimit(kind: keyof typeof HISTORY_LIMITS, value: number): HistoryLimitError | null {
  if (!Number.isInteger(value)) return "integer";
  const { min, max } = HISTORY_LIMITS[kind];
  return value < min || value > max ? "range" : null;
}

/** Newest events first; keeps the newest `max`. */
export function pruneEvents(events: readonly OpsEvent[], max: number): OpsEvent[] {
  return events.length > max ? events.slice(0, Math.max(0, max)) : [...events];
}

/**
 * Every running run stays (the run gate, toasts and the action dialog still
 * follow them); of the finished ones the `maxFinished` with the latest finish
 * time stay (start time for runs without one). The order of the list is kept.
 */
export function pruneRuns(runs: readonly ActionRun[], maxFinished: number): ActionRun[] {
  const finished = runs.filter((r) => r.status !== "running" && r.status !== "queued");
  if (finished.length <= maxFinished) return [...runs];
  const stamp = (r: ActionRun) => Date.parse(r.finishedAt ?? r.startedAt);
  // Stable sort: equal stamps keep list order, so the earlier (newer-started) entry wins.
  const keep = new Set(finished.map((r, i) => ({ r, i })).sort((a, b) => stamp(b.r) - stamp(a.r) || a.i - b.i).slice(0, Math.max(0, maxFinished)).map((e) => e.r));
  return runs.filter((r) => r.status === "running" || r.status === "queued" || keep.has(r));
}
