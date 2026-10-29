// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification target and per-host Auto Heal queries; secrets never enter caches.

import { useQuery } from "@tanstack/react-query";
import { useAdapter } from "./use-adapter";

export const notificationKeys = { targets: ["notification-targets"] as const, autoHeal: (hostId: string) => ["auto-heal", hostId] as const };

export function useNotificationTargets() {
  const adapter = useAdapter();
  return useQuery({ queryKey: notificationKeys.targets, queryFn: () => adapter.listNotificationTargets() });
}
export function useAutoHeal(hostId: string, enabled: boolean) {
  const adapter = useAdapter();
  return useQuery({ queryKey: notificationKeys.autoHeal(hostId), queryFn: () => adapter.getAutoHealSettings(hostId), enabled, retry: false });
}
