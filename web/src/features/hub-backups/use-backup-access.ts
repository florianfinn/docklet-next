// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original Hub backup permission query for issue #32.

import { useQuery } from "@tanstack/react-query";

import { useAdapter } from "@/data/use-adapter";

export function useHubBackupAccess() {
  const adapter = useAdapter();
  return useQuery({ queryKey: ["hub-backup-access"], queryFn: () => adapter.getHubBackupAccess(), staleTime: 30_000 });
}
