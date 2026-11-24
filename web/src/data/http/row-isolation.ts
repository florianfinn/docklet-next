// Source: docklet-next/issues/600
// Revision: 393e645a79236449c35cb8e1a4827c83bbf45da0
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original per-row isolation collector for host projections.

// Per-row isolation for host projections: one row that cannot be mapped is left out and counted,
// every other row of the host stays (#600). Collectors are created per read; nothing shared is mutated.

import type { UnreadableKind, UnreadableRows } from "@/domain/model";

const CODE = /^[a-z][a-z0-9-]*(\.[a-z0-9-]+)+$/;
const MAX_NAMES = 8;

export interface RowCollector {
  readonly kind: UnreadableKind;
  /** Maps one row; a throw is recorded for this row only. `name` labels the row in the notice when it is readable. */
  one<T, R>(row: T, map: (row: T) => R, name?: (row: T) => unknown): R | undefined;
  each<T, R>(rows: readonly T[], map: (row: T) => R, name?: (row: T) => unknown): R[];
  result(): UnreadableRows;
}

export function rowCollector(kind: UnreadableKind): RowCollector {
  let count = 0;
  const codes: string[] = [];
  const names: string[] = [];
  const collector: RowCollector = {
    kind,
    one(row, map, name) {
      try { return map(row); }
      catch (error) {
        count += 1;
        const code = error instanceof Error && CODE.test(error.message) ? error.message : "inventory.mapping-error";
        if (!codes.includes(code)) codes.push(code);
        let label: unknown;
        try { label = name?.(row); } catch { label = undefined; }
        if (typeof label === "string" && label.length > 0 && label.length <= 128 && names.length < MAX_NAMES && !names.includes(label)) names.push(label);
        return undefined;
      }
    },
    each(rows, map, name) {
      const out: ReturnType<typeof map>[] = [];
      for (const row of rows) {
        const value = collector.one(row, map, name);
        if (value !== undefined) out.push(value);
      }
      return out;
    },
    result: () => ({ count, codes: [...codes], names: [...names] }),
  };
  return collector;
}

/** A new observation carrying the collectors' results; the input observation is not changed. */
export function withUnreadable<O extends { unreadable?: Partial<Record<UnreadableKind, UnreadableRows>> }>(observation: O, ...collectors: RowCollector[]): O {
  const unreadable = { ...observation.unreadable };
  for (const c of collectors) unreadable[c.kind] = c.result();
  return { ...observation, unreadable };
}
