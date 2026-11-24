// Source: docklet-next/issues/612
// Revision: 20b1ec05bcffb69db599d4fa60f0f37ffcbfdb2b
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original M39 #612 result lines for collapsed Action Centre entries.

import { useQueries, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef } from "react";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import type { ActionRun } from "@/domain/model";
import type { ScanRunSummary } from "@/domain/security";
import { SCAN_RUN_LIST_LIMIT } from "@/data/http/vulnerability-runs";
import { isHostScan } from "./scan-counts";

const settled = (run: ActionRun) => run.status !== "running" && run.status !== "queued";

/**
 * A settled run belongs in its host's list when the list is not full or the run started after the list's oldest
 * entry. Missing such a run means the list was read before the run settled (a first load does not repeat on a
 * change), so the list is read once more for that set of runs.
 */
function missingListed(runs: readonly ActionRun[], list: ReadonlyMap<string, ScanRunSummary>): string[] {
  const oldest = list.size < SCAN_RUN_LIST_LIMIT ? -Infinity : Math.min(...[...list.values()].map((s) => Date.parse(s.startedAt)));
  return runs.filter((r) => !list.has(r.id) && Date.parse(r.startedAt) >= oldest).map((r) => r.id);
}

/**
 * One list read per host that has settled host scans in view; the lookup only reads the shared,
 * frozen results. Runs outside a host's newest 30 have no entry here and get their line from the
 * expanded detail instead.
 */
export function useScanRunLookup(runs: readonly ActionRun[]): (run: ActionRun) => ScanRunSummary | undefined {
  const adapter = useAdapter(), client = useQueryClient();
  const scans = useMemo(() => runs.filter((r) => isHostScan(r) && settled(r)), [runs]);
  const hosts = useMemo(() => [...new Set(scans.map((r) => r.hostId ?? "0"))].sort(), [scans]);
  const results = useQueries({
    queries: hosts.map((hostId) => ({
      queryKey: queryKeys.scanRuns(hostId),
      enabled: !!adapter.getScanRunSummaries,
      queryFn: ({ signal }: { signal: AbortSignal }) => adapter.getScanRunSummaries!(hostId, signal),
      staleTime: 30_000,
    })),
  });
  const byHost = new Map(hosts.map((hostId, index) => [hostId, results[index]?.data?.runs]));
  const retried = useRef(new Set<string>());
  const stale = hosts.flatMap((hostId) => {
    const list = byHost.get(hostId), result = results[hosts.indexOf(hostId)];
    if (!list || result?.isFetching || result?.data?.unavailable) return [];
    const missing = missingListed(scans.filter((r) => (r.hostId ?? "0") === hostId), list);
    return missing.length ? [`${hostId}\n${missing.join(",")}`] : [];
  }).join("\n\n");
  useEffect(() => {
    for (const entry of stale ? stale.split("\n\n") : []) {
      if (retried.current.has(entry)) continue;
      retried.current.add(entry);
      void client.invalidateQueries({ queryKey: queryKeys.scanRuns(entry.split("\n")[0]!), exact: true });
    }
  }, [client, stale]);
  return (run) => (isHostScan(run) && settled(run) ? byHost.get(run.hostId ?? "0")?.get(run.id) : undefined);
}
