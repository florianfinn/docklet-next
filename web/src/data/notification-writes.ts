// Source: docklet-next/issues/297
// Revision: 693beb34e4f331cd918c2183f07b2f935a8620e7
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original adapter-scoped notification write ownership.

import { CancelledError, useQueryClient } from "@tanstack/react-query";
import { useCallback, useSyncExternalStore } from "react";

import type { NotificationProvider } from "@/domain/notifications";

import type { DockletAdapter } from "./adapter";
import { notificationKeys } from "./notification-queries";
import { useAdapter } from "./use-adapter";

interface Writes {
  pending: ReadonlySet<NotificationProvider>;
  listeners: Set<() => void>;
}
const owners = new WeakMap<object, Writes>();
// Keyed like the identity lifetime: the subscription function survives the RunGate proxy unchanged.
function writes(adapter: DockletAdapter): Writes {
  let state = owners.get(adapter.subscribeChanges);
  if (!state) owners.set(adapter.subscribeChanges, (state = { pending: new Set(), listeners: new Set() }));
  return state;
}
function mark(state: Writes, provider: NotificationProvider, on: boolean) {
  const next = new Set(state.pending);
  if (on) next.add(provider);
  else next.delete(provider);
  state.pending = next;
  for (const notify of state.listeners) notify();
}

/**
 * One full-target write per provider and adapter until it and the following list refresh settle.
 * Ownership outlives dialogs and section mounts; a second write for a held provider is refused.
 */
export function useNotificationWrites() {
  const adapter = useAdapter();
  const client = useQueryClient();
  const state = writes(adapter);
  const subscribe = useCallback((notify: () => void) => {
    state.listeners.add(notify);
    return () => { state.listeners.delete(notify); };
  }, [state]);
  const pending = useSyncExternalStore(subscribe, () => state.pending, () => state.pending);
  const write = async <T,>(provider: NotificationProvider, run: () => Promise<T>): Promise<T> => {
    if (state.pending.has(provider)) throw new CancelledError();
    mark(state, provider, true);
    try {
      return await run();
    } finally {
      await client.refetchQueries({ queryKey: notificationKeys.targets }).catch(() => undefined);
      mark(state, provider, false);
    }
  };
  return { pending, write };
}
