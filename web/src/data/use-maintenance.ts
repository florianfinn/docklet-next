// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original React binding of the adapter's hub maintenance observation.

import { useSyncExternalStore } from "react";

import type { MaintenanceState } from "./http/maintenance-observer";
import { useAdapter } from "./use-adapter";

const IDLE: MaintenanceState = { active: false, overdue: false };
const none = () => () => {};

/** Hub maintenance as the transport observed it; adapters without the port never report maintenance. */
export function useMaintenance(): MaintenanceState {
  const port = useAdapter().maintenance;
  return useSyncExternalStore(port?.subscribe ?? none, port?.state ?? (() => IDLE), () => IDLE);
}
