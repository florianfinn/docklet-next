// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original React hook over the hub update session store.


import { useSyncExternalStore } from "react";
import { useAdapter } from "@/data/use-adapter";
import type { HubUpdateSnapshot } from "@/data/http/hub-update-session";

const idle: HubUpdateSnapshot = { state: { phase: "idle" }, dismissed: false };
const noop = () => () => {};
export function useHubUpdate() {
  const session = useAdapter().hubUpdateSession;
  return useSyncExternalStore(session?.subscribe ?? noop, session?.get ?? (() => idle));
}
