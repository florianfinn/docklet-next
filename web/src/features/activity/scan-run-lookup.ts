// Source: docklet-next/issues/612
// Revision: 20b1ec05bcffb69db599d4fa60f0f37ffcbfdb2b
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original M39 #612 result lines for collapsed Action Centre entries.

import { useQueries } from "@tanstack/react-query";
import { useMemo } from "react";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import type { ActionRun } from "@/domain/model";
import type { ScanRunSummary } from "@/domain/security";
import { isHostScan } from "./scan-counts";

const settled = (run: ActionRun) => run.status !== "running" && run.status !== "queued";

/**
 * One list read per host that has settled host scans in view; the lookup only reads the shared,
 * frozen results. Runs outside a host's newest 30 have no entry here and get their line from the
 * expanded detail instead.
 */
export function useScanRunLookup(runs: readonly ActionRun[]): (run: ActionRun) => ScanRunSummary | undefined {
  const adapter = useAdapter();
  const hosts = useMemo(() => [...new Set(runs.filter((r) => isHostScan(r) && settled(r)).map((r) => r.hostId ?? "0"))].sort(), [runs]);
  const results = useQueries({
    queries: hosts.map((hostId) => ({
      queryKey: queryKeys.scanRuns(hostId),
      enabled: !!adapter.getScanRunSummaries,
      queryFn: ({ signal }: { signal: AbortSignal }) => adapter.getScanRunSummaries!(hostId, signal),
      staleTime: 30_000,
    })),
  });
  const byHost = new Map(hosts.map((hostId, index) => [hostId, results[index]?.data?.runs]));
  return (run) => (isHostScan(run) && settled(run) ? byHost.get(run.hostId ?? "0")?.get(run.id) : undefined);
}
