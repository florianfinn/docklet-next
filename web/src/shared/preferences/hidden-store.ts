import { createStore, useStore } from "@/shared/lib/store";

/**
 * Presentation preference: stacks and containers hidden from their overview.
 * Hiding never stops, removes or changes anything; detail pages, counts and
 * actions keep working, and every hidden entry stays restorable per host.
 */
export type HiddenKind = "stacks" | "containers";
export type HiddenItems = Record<HiddenKind, string[]>;

const EMPTY: HiddenItems = { stacks: [], containers: [] };

const ids = (v: unknown): string[] => (Array.isArray(v) ? [...new Set(v.filter((x): x is string => typeof x === "string" && x.length > 0))] : []);

export function mergeHidden(stored: unknown): HiddenItems {
  const s = (stored && typeof stored === "object" ? stored : {}) as Partial<Record<HiddenKind, unknown>>;
  return { stacks: ids(s.stacks), containers: ids(s.containers) };
}

export const hiddenStore = createStore<HiddenItems>(EMPTY, { key: "docklet-react-opus.hidden", version: 1, merge: (stored) => mergeHidden(stored) });

export function useHiddenIds(kind: HiddenKind): string[] {
  return useStore(hiddenStore, (h) => h[kind]);
}

export function setHidden(kind: HiddenKind, id: string, hidden: boolean): void {
  hiddenStore.set((h) => ({ ...h, [kind]: hidden ? [...new Set([...h[kind], id])] : h[kind].filter((x) => x !== id) }));
}

export function restoreAll(kind: HiddenKind, list: readonly string[]): void {
  hiddenStore.set((h) => ({ ...h, [kind]: h[kind].filter((x) => !list.includes(x)) }));
}

/** Splits rows into shown and hidden ones; both keep their order. */
export function splitHidden<T extends { id: string }>(rows: readonly T[], hidden: readonly string[]): { shown: T[]; hidden: T[] } {
  if (!hidden.length) return { shown: [...rows], hidden: [] };
  const set = new Set(hidden);
  return { shown: rows.filter((r) => !set.has(r.id)), hidden: rows.filter((r) => set.has(r.id)) };
}
