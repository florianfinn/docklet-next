// Source: docklet-next/issues/280
// Revision: 5a4ecc67a7712349cb869574429d09f09a96e895
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original image overview column sets, extracted so every layout keeps the findings column.

import type { OverviewColumn, TableView } from "@/shared/components/overview/overview-table";

import type { ImageSortKey } from "./image-filters";

type ColumnKey = ImageSortKey | "security";

/**
 * Column sets per table layout; cards reuse the full set. Both table layouts
 * keep the findings column so findings stay visible however the view fits.
 */
export function imageColumns(label: (column: ColumnKey) => string, inBlock: boolean, view: TableView): OverviewColumn<ImageSortKey>[] {
  const key = (k: ImageSortKey) => ({ key: k, label: label(k) });
  const security = { id: "security", label: label("security") };
  if (view === "condensed")
    return [
      { id: "name", label: label("name"), sorts: [key("name"), key("version"), key("usage"), ...(inBlock ? [] : [key("host")])], className: "min-w-52" },
      { id: "consumers", label: label("consumers"), sorts: [key("consumers")], className: "max-w-72", wide: true },
      { id: "meta", label: label("size"), sorts: [key("size"), key("created")] },
      { id: "update", label: label("update"), sorts: [key("update")], wide: true },
      security,
    ];
  return [
    { id: "name", label: label("name"), sorts: [key("name")], className: "min-w-52" },
    { id: "version", label: label("version"), sorts: [key("version")] },
    ...(inBlock ? [] : [{ id: "host", label: label("host"), sorts: [key("host")] }]),
    { id: "usage", label: label("usage"), sorts: [key("usage")] },
    { id: "consumers", label: label("consumers"), sorts: [key("consumers")], className: "max-w-72", wide: true },
    { id: "size", label: label("size"), sorts: [key("size")] },
    { id: "created", label: label("created"), sorts: [key("created")] },
    { id: "update", label: label("update"), sorts: [key("update")], wide: true },
    security,
  ];
}
