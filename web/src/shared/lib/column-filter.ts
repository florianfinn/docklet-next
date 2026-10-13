/** Per-column table filters: free text ("contains") or one choice of a column's own values. */
export type ColumnFilter<T> =
  | { id: string; kind: "text"; text: (row: T) => string }
  | { id: string; kind: "choice"; options: { value: string; label: string }[]; match: (row: T, value: string) => boolean };

export type FilterValues = Record<string, string>;

/** Rows that pass every set filter (conjunction); empty values do not filter. */
export function applyColumnFilters<T>(rows: readonly T[], filters: readonly ColumnFilter<T>[], values: FilterValues): T[] {
  const active = filters.filter((f) => (values[f.id] ?? "").trim() !== "");
  if (!active.length) return [...rows];
  return rows.filter((row) =>
    active.every((f) => {
      const v = (values[f.id] ?? "").trim();
      return f.kind === "text" ? f.text(row).toLowerCase().includes(v.toLowerCase()) : f.match(row, v);
    }),
  );
}

export function activeFilters(values: FilterValues): number {
  return Object.values(values).filter((v) => v.trim() !== "").length;
}
