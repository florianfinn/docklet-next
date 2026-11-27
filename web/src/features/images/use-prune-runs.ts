// Source: docklet-next/docs/design/image-prune-progress.md
// Revision: b637b8c19497e52b7a62987a497ca4ee05b1b5af
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original cleanup run binding and count text for issue #661.

import { useState } from "react";
import { useTranslations } from "use-intl";
import type { ActionRun, BulkCheckCounts } from "@/domain/model";
import { useActionRuns } from "@/data/live";
import { formatBulkPruneCounts } from "@/features/activity/bulk-check-counts";

const isPrune = (run: ActionRun) => run.sourceOperation === "image_prune";

/** Run ids present before a submit; only runs absent from it can belong to that submit. */
export function usePruneRunSnapshot(): () => ReadonlySet<string> {
  const runs = useActionRuns();
  return () => new Set(runs.filter(isPrune).map((run) => run.id));
}

interface Bindings { before?: ReadonlySet<string>; ids: ReadonlyMap<string, string>; last: ReadonlyMap<string, ActionRun> }
const EMPTY_BINDINGS: Bindings = { ids: new Map(), last: new Map() };
const sameMap = <V,>(a: ReadonlyMap<string, V>, b: ReadonlyMap<string, V>) => a.size === b.size && [...a].every(([key, value]) => b.get(key) === value);

/**
 * Binds each host to the first new image_prune run observed for it while the submit's request is
 * pending. A host runs one cleanup at a time (the image lease), so that run is this submit's; a
 * binding never moves, and once the request settled no new run is bound. The last valid counts of
 * a bound run stay with this submit even when the run later leaves the list or loses its counts.
 */
export function usePruneRuns(before: ReadonlySet<string> | undefined, hostIds: readonly string[], admitting: boolean): ReadonlyMap<string, ActionRun> {
  const runs = useActionRuns();
  const [bound, setBound] = useState<Bindings>(EMPTY_BINDINGS);
  const base = bound.before === before ? bound : { ...EMPTY_BINDINGS, before };
  const ids = new Map(base.ids), last = new Map(base.last), out = new Map<string, ActionRun>();
  if (before) for (const hostId of hostIds) {
    const id = ids.get(hostId);
    const found = id ? runs.find((r) => r.id === id) : admitting ? runs.filter((r) => isPrune(r) && r.hostId === hostId && !before.has(r.id)).sort((a, b) => a.startedAt.localeCompare(b.startedAt))[0] : undefined;
    if (found) ids.set(hostId, found.id);
    if (found?.bulk) last.set(hostId, found);
    const run = found?.bulk ? found : last.get(hostId) ?? found;
    if (run) out.set(hostId, run);
  }
  // Bindings are adjusted during render, like derived state; an unchanged binding sets nothing.
  if (base !== bound || !sameMap(ids, bound.ids) || !sameMap(last, bound.last)) setBound({ before, ids, last });
  return out;
}

export function usePruneCountText(): (bulk: BulkCheckCounts, running: boolean) => string {
  const t = useTranslations("activity.bulk");
  return (bulk, running) => formatBulkPruneCounts(bulk, running, (key, values) => t(key === "processed" || key === "removed" ? `prune.${key}` : key, values));
}
