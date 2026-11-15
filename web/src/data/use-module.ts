// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original module visibility and identity-owned asynchronous reads.

import { useEffect, useSyncExternalStore } from "react";
import { CancelledError, useQuery, useQueryClient, type QueryKey } from "@tanstack/react-query";
import { defaultModules, type ModuleID } from "@/domain/modules";
import { identityLifetime } from "./identity-lifetime";
import { useAdapter } from "./use-adapter";
import { useIsUser } from "./use-principal";

export function useIdentityVersion() {
  const lifetime = identityLifetime(useAdapter());
  return useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
}
export function useModuleStates() {
  const adapter = useAdapter(), version = useIdentityVersion(), user = useIsUser();
  return useQuery({ queryKey: ["module-states", version], queryFn: async ({ signal }) => {
    const value = await adapter.getModuleStates!(signal);
    signal.throwIfAborted();
    if (version !== identityLifetime(adapter).version()) throw new CancelledError();
    return value;
  }, enabled: !user && !!adapter.getModuleStates });
}
export function useModuleEnabled(id: ModuleID) {
  const states = useModuleStates(), user = useIsUser();
  return !user && (states.data ?? defaultModules())[id];
}

/** Module consumers use this hook so late reads cannot revive disabled surfaces
 * or write data into another principal's cache. Reads must honour the signal. */
export function useModuleRead<T>(id: ModuleID, key: QueryKey, read: (signal: AbortSignal) => Promise<T>) {
  const adapter = useAdapter(), client = useQueryClient(), version = useIdentityVersion();
  const enabled = useModuleEnabled(id), stateKey = ["module-states", version];
  const root = ["module", id, version];
  useEffect(() => {
    if (!enabled) void client.cancelQueries({ queryKey: ["module", id, version] });
  }, [client, enabled, id, version]);
  const query = useQuery({ queryKey: [...root, ...key], enabled, queryFn: async ({ signal }) => {
    const value = await read(signal);
    signal.throwIfAborted();
    const states = client.getQueryData<ReturnType<typeof defaultModules>>(stateKey);
    if (identityLifetime(adapter).version() !== version || !states?.[id]) throw new CancelledError();
    return value;
  } });
  return { ...query, data: enabled ? query.data : undefined };
}
