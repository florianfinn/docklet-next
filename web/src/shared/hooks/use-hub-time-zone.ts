// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/shared/hooks/use-hub-time-zone.ts
// Revision: a860bd8643dd055f3e6729205c71c89e30d5537c89b43e50f90f57f0c7c37084
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: #69 connected identity and hub settings. Simple-user management, assigned-stack access and mode-aware self-service.

import { useConnectedSettings, useSettings } from "@/data/queries";
import { usePrincipal } from "@/data/use-principal";
import { useAdapter } from "@/data/use-adapter";
import { DEFAULT_TIME_ZONE } from "@/domain/schedule";

/** The process TZ drives connected schedules and display; demo data carries its own zone. */
export function useHubTimeZone(): string {
  const connected = useAdapter().kind === "http";
  const settings = useSettings(!connected);
  const principal = usePrincipal();
  const hub = useConnectedSettings(connected && !!principal.data && principal.data.isAdmin !== false);
  return (connected ? hub.data?.general.timeZone : settings.data?.general.timeZone) || (connected ? "UTC" : DEFAULT_TIME_ZONE);
}
