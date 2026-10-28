import { QueryClient, QueryClientProvider, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState, type ReactNode } from "react";

import { resetActionIdentity } from "@/features/activity/run-presentation";
import { actionDialogStore } from "@/features/activity/action-dialog-store";

import type { ContainerInfo } from "@/domain/model";

import type { DockletAdapter, Topic } from "./adapter";
import { runGateOf, withRunGate } from "./run-gate";
import { AdapterContext } from "./use-adapter";
import { identityLifetime } from "./identity-lifetime";
import { registerReadConsumer } from "./read-consumers";
import { queryKeys } from "./query-keys";
import { useCurrentUser } from "./queries";

const TOPIC_KEYS: Record<Topic, readonly (readonly unknown[])[]> = {
  hosts: [queryKeys.hosts, queryKeys.systemComponents],
  // Images derive their consumers from containers, so container and stack changes refresh them too.
  stacks: [queryKeys.stacks, queryKeys.stackRoot, queryKeys.images],
  containers: [queryKeys.containers, queryKeys.images],
  security: [queryKeys.security],
  topology: [queryKeys.topology],
  events: [queryKeys.events],
  schedule: [queryKeys.schedule],
  volumes: [queryKeys.volumes, queryKeys.volumeRoot, queryKeys.fileRoot, ["backup-overviews"], ["backup-overview"], ["backup-repository"]],
  settings: [queryKeys.settings, queryKeys.connectedSettings, ["backup-targets"], ["backup-repository"], ["backup-overviews"], ["backup-overview"]],
  images: [queryKeys.images],
  registries: [["registries"]],
};

/** Invalidates cached reads when the adapter reports changes (SSE-like). */
function ChangeBridge({ adapter }: { adapter: DockletAdapter }) {
  const client = useQueryClient();
  const identity = useCurrentUser(adapter.kind === "http");
  const enabled = adapter.kind !== "http" || (identity.isSuccess && !!identity.data);
  useEffect(
    () => {
      if (!enabled) return;
      const offDemand = registerReadConsumer(adapter.subscribeChanges, "events", () => client.getQueryCache().find({ queryKey: queryKeys.events, exact: true })?.isActive() ?? false);
      const offChanges = adapter.subscribeChanges((topics) => {
        for (const topic of new Set(topics)) {
          for (const key of TOPIC_KEYS[topic]) void client.invalidateQueries({ queryKey: key });
        }
      });
      return () => { offDemand(); offChanges(); };
    },
    [adapter, client, enabled],
  );
  return null;
}

function IdentityBridge({ adapter }: { adapter: DockletAdapter }) {
  const client = useQueryClient();
  useEffect(() => {
    const lifetime = identityLifetime(adapter);
    return lifetime.listen(() => {
      const version = lifetime.version();
      // Active notifications come from a confirmed identity; preserve its query and result.
      const preserveCurrentUser = lifetime.active();
      const cancel = client.cancelQueries({ predicate: (query) => query.queryKey[0] !== queryKeys.authMode[0] && (!preserveCurrentUser || query.queryKey[0] !== queryKeys.currentUser[0]) }, { revert: false });
      const clear = () => {
        for (const query of client.getQueryCache().findAll()) {
          if (query.queryKey[0] !== queryKeys.authMode[0] && query.queryKey[0] !== queryKeys.currentUser[0]) query.reset();
        }
        if (!preserveCurrentUser) client.setQueryData(queryKeys.currentUser, null);
      };
      resetActionIdentity(); actionDialogStore.set({ waiting: [] });
      clear();
      // Cancellation must settle before fresh observers reuse the same query objects.
      void cancel.then(() => {
        if (version !== lifetime.version()) return;
        clear();
        if (lifetime.active()) void client.invalidateQueries({ predicate: (query) => query.queryKey[0] !== queryKeys.authMode[0] && (!preserveCurrentUser || query.queryKey[0] !== queryKeys.currentUser[0]) });
      });
    });
  }, [adapter, client]);
  return null;
}

/** Tells the run gate which stack a container belongs to, from cached container data. */
function GateParents({ adapter }: { adapter: DockletAdapter }) {
  const client = useQueryClient();
  useEffect(
    () =>
      runGateOf(adapter)?.setParentResolver((scope) => {
        if (!scope.startsWith("container:")) return undefined;
        const id = scope.slice("container:".length);
        const stackId = client.getQueryData<ContainerInfo[]>(queryKeys.containers)?.find((c) => c.id === id)?.stackId;
        return stackId ? `stack:${stackId}` : undefined;
      }),
    [adapter, client],
  );
  return null;
}

/** Provides the adapter behind the run gate, so no page can start the same run twice. */
export function AdapterProvider({ adapter: inner, children }: { adapter: DockletAdapter; children: ReactNode }) {
  const adapter = useMemo(() => withRunGate(inner), [inner]);
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: { retry: false, refetchOnWindowFocus: false, staleTime: 30_000 },
          mutations: { retry: false },
        },
      }),
  );
  return (
    <AdapterContext.Provider value={adapter}>
      <QueryClientProvider client={client}>
        <IdentityBridge adapter={adapter} />
        <ChangeBridge adapter={adapter} />
        <GateParents adapter={adapter} />
        {children}
      </QueryClientProvider>
    </AdapterContext.Provider>
  );
}
