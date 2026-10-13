import { COLLATOR } from "@/domain/hosts";

export type SortDir = "asc" | "desc";

export interface SortState<K extends string> {
  key: K;
  dir: SortDir;
}

/**
 * Value used for one column; strings use the shared German collator, numbers
 * compare numerically. `null` means unknown (e.g. no live load for a stopped
 * container) and always sorts last, in both directions.
 */
export type SortValue = string | number | null;

export interface RowIdentity {
  id: string;
  name: string;
  hostName: string;
}

function compareValues(a: Exclude<SortValue, null>, b: Exclude<SortValue, null>): number {
  if (typeof a === "number" && typeof b === "number") return a - b;
  return COLLATOR.compare(String(a), String(b));
}

/**
 * Sorts rows by one column and always breaks ties the same way: host name
 * ascending, then name ascending, then id. Only the chosen column follows the
 * direction, so "host descending" still lists names A–Z within each host.
 */
export function sortRows<T, K extends string>(rows: readonly T[], sort: SortState<K>, value: (row: T, key: K) => SortValue, identity: (row: T) => RowIdentity): T[] {
  const sign = sort.dir === "asc" ? 1 : -1;
  return rows.toSorted((ra, rb) => {
    const va = value(ra, sort.key);
    const vb = value(rb, sort.key);
    const primary = va === null || vb === null ? (va === null ? 1 : 0) - (vb === null ? 1 : 0) : compareValues(va, vb) * sign;
    if (primary) return primary;
    const a = identity(ra);
    const b = identity(rb);
    return COLLATOR.compare(a.hostName, b.hostName) || COLLATOR.compare(a.name, b.name) || COLLATOR.compare(a.id, b.id);
  });
}

/** Clicking the active column flips it; another column starts with its default direction. */
export function nextSort<K extends string>(current: SortState<K>, key: K, defaultDir: (key: K) => SortDir = () => "asc"): SortState<K> {
  if (current.key === key) return { key, dir: current.dir === "asc" ? "desc" : "asc" };
  return { key, dir: defaultDir(key) };
}

export function ariaSort(sort: SortState<string>, key: string | readonly string[]): "ascending" | "descending" | "none" {
  const keys: readonly string[] = typeof key === "string" ? [key] : key;
  return !keys.includes(sort.key) ? "none" : sort.dir === "asc" ? "ascending" : "descending";
}
