// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers.

import { useQuery } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";
import { useAdapter } from "./use-adapter";
import { usePrincipal } from "./use-principal";

const empty = () => undefined;
const noSubscribe = () => () => {};

/** Hub-wide facts remain admin-only; all surfaces share the same stream observation. */
export function useTailnet() {
  const adapter = useAdapter();
  const principal = usePrincipal();
  const api = principal.data?.isAdmin === true ? adapter.tailnet : undefined;
  const status = useSyncExternalStore(api?.subscribe ?? noSubscribe, api?.current ?? empty, empty);
  const query = useQuery({ queryKey: ["tailnet-status"], queryFn: () => api!.status(), enabled: !!api, retry: false });
  return { api, status, query };
}
