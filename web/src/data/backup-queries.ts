// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original host-scoped backup queries; recovery keys never enter query caches.

import { type QueryClient, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReadTag } from "./http/host-read-policy";
import { useAdapter } from "./use-adapter";
import { attemptCurrent } from "./refresh-owner";
import type { BackupOverview } from "@/domain/volumes";

/**
 * Only an interval refresh of loaded data is a poll (skipped for standby hosts, Q-5a). A first load, an invalidation
 * and a remount after older data are background reads; an interval refresh finds data about one interval old.
 */
function readTag(client: QueryClient, key: readonly unknown[], intervalMs: number): ReadTag {
  const state = client.getQueryState(key);
  if (state?.data === undefined || state.isInvalidated) return "background";
  return Date.now() - state.dataUpdatedAt <= 2 * intervalMs ? "poll" : "background";
}

export function useBackupOverview(host: string, enabled = true) {
  const adapter = useAdapter(),
    client = useQueryClient();
  return useQuery({
    queryKey: ["backup-overview", host],
    queryFn: () => adapter.getBackupOverview(host, readTag(client, ["backup-overview", host], 5000)),
    enabled: enabled && !!host,
    refetchInterval: 5000,
  });
}
export function useBackupOverviews() {
  const adapter = useAdapter(),
    client = useQueryClient();
  return useQuery({
    queryKey: ["backup-overviews"],
    queryFn: ({ signal }) => {
      // A superseded attempt publishes no partial rows (refresh-owner.ts).
      const current = attemptCurrent(client, ["backup-overviews"]);
      return adapter.listBackupOverviews((rows) => {
        if (!signal.aborted && current() && client.getQueryData<BackupOverview[]>(["backup-overviews"]) === undefined)
          client.setQueryData(["backup-overviews"], rows);
      }, readTag(client, ["backup-overviews"], 10000));
    },
    refetchInterval: 10000,
  });
}
export function useBackupTargets(enabled = true) {
  const adapter = useAdapter();
  return useQuery({ queryKey: ["backup-targets"], queryFn: () => adapter.listBackupTargets(), enabled });
}
export function useBackupDefaultRetention() {
  const adapter = useAdapter();
  return useQuery({ queryKey: ["backup-default-retention"], queryFn: () => adapter.getBackupDefaultRetention?.() ?? 7 });
}
export function useBackupRepository(host: string, enabled = true) {
  const adapter = useAdapter(),
    client = useQueryClient();
  return useQuery({
    queryKey: ["backup-repository", host],
    queryFn: () => adapter.getBackupRepository(host, readTag(client, ["backup-repository", host], 5000)),
    enabled,
    refetchInterval: 5000,
  });
}
