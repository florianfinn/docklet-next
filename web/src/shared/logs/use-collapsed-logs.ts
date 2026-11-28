// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original M47 Health and Logs implementation and deterministic regression evidence.

import { useMemo, useState } from "react";
import type { LogLine } from "@/domain/model";
import { collapseLogs } from "./derive";

export function useCollapsedLogs(lines: LogLine[]) {
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const groups = useMemo(() => collapseLogs(lines), [lines]);
  const rows = groups.flatMap((group) => expanded.has(group[0]!.id) ? group : [group[0]!]);
  const counts = new Map(groups.filter((g) => g.length > 1).map((g) => [g[0]!.id, g.length]));
  return { rows, counts, expanded, toggle(id: string) { setExpanded((old) => { const next = new Set(old); if (next.has(id)) next.delete(id); else next.add(id); return next; }); }, reveal(id: string) {
    const group = groups.find((g) => g.some((l) => l.id === id));
    if (group && group[0]!.id !== id) setExpanded((old) => new Set([...old, group[0]!.id]));
  } };
}
