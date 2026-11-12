// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/shared/hooks/use-hub-time-zone.ts
// Revision: a860bd8643dd055f3e6729205c71c89e30d5537c89b43e50f90f57f0c7c37084
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: #69 connected identity and hub settings. Simple-user management, assigned-stack access and mode-aware self-service. M18 #341 #343 #342 #344 #250 #346 #319: v14 policies, explicit update/recovery gestures and shared scheduling rule. #410 hub zone for simple users.

import { useQuery } from "@tanstack/react-query";
import { useConnectedSettings, useSettings } from "@/data/queries";
import { queryKeys } from "@/data/query-keys";
import { usePrincipal } from "@/data/use-principal";
import { useAdapter } from "@/data/use-adapter";
import { DEFAULT_TIME_ZONE } from "@/domain/schedule";

/** Effective hub settings drive schedules and display; never infer the browser zone. */
export function useHubTimeZone(): string {
  const adapter = useAdapter();
  const connected = adapter.kind === "http";
  const settings = useSettings(!connected);
  const principal = usePrincipal();
  const hub = useConnectedSettings(connected && !!principal.data && principal.data.isAdmin !== false);
  // Simple users cannot read connected settings; they read only the effective zone.
  const user = useQuery({ queryKey: [...queryKeys.connectedSettings, "hub-time-zone"], queryFn: () => adapter.getHubTimeZone!(), enabled: connected && principal.data?.isAdmin === false && !!adapter.getHubTimeZone });
  const zone = principal.data?.isAdmin === false ? user.data : hub.data?.general.timeZone;
  return (connected ? zone : settings.data?.general.timeZone) || (connected ? "UTC" : DEFAULT_TIME_ZONE);
}
