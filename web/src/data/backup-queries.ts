// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original host-scoped backup queries; recovery keys never enter query caches.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAdapter } from "./use-adapter";
import type { BackupOverview } from "@/domain/volumes";

export function useBackupOverview(host: string, enabled = true) {
  const adapter = useAdapter();
  return useQuery({ queryKey: ["backup-overview", host], queryFn: () => adapter.getBackupOverview(host), enabled: enabled && !!host, refetchInterval: 5000 });
}
export function useBackupOverviews() {
  const adapter = useAdapter(), client = useQueryClient();
  return useQuery({ queryKey: ["backup-overviews"], queryFn: ({ signal }) => adapter.listBackupOverviews((rows) => { if (!signal.aborted && client.getQueryData<BackupOverview[]>(["backup-overviews"]) === undefined) client.setQueryData(["backup-overviews"], rows); }), refetchInterval: 10000 });
}
export function useBackupTargets(enabled = true) {
  const adapter = useAdapter();
  return useQuery({ queryKey: ["backup-targets"], queryFn: () => adapter.listBackupTargets(), enabled });
}
export function useBackupRepository(host: string) {
  const adapter = useAdapter();
  return useQuery({ queryKey: ["backup-repository", host], queryFn: () => adapter.getBackupRepository(host), refetchInterval: 5000 });
}
