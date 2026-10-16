import { QueryClient, QueryClientProvider, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState, type ReactNode } from "react";

import type { ContainerInfo } from "@/domain/model";

import type { DockletAdapter, Topic } from "./adapter";
import { runGateOf, withRunGate } from "./run-gate";
import { AdapterContext } from "./use-adapter";
import { queryKeys } from "./query-keys";

const TOPIC_KEYS: Record<Topic, readonly (readonly unknown[])[]> = {
  hosts: [queryKeys.hosts, queryKeys.systemComponents],
  // Images derive their consumers from containers, so container and stack changes refresh them too.
  stacks: [queryKeys.stacks, queryKeys.stackRoot, queryKeys.images],
  containers: [queryKeys.containers, queryKeys.images],
  security: [queryKeys.security],
  topology: [queryKeys.topology],
  events: [queryKeys.events],
  schedule: [queryKeys.schedule],
  volumes: [queryKeys.volumes, queryKeys.volumeRoot, queryKeys.fileRoot],
  settings: [queryKeys.settings, queryKeys.connectedSettings],
  images: [queryKeys.images],
};

/** Invalidates cached reads when the adapter reports changes (SSE-like). */
function ChangeBridge({ adapter }: { adapter: DockletAdapter }) {
  const client = useQueryClient();
  useEffect(
    () =>
      adapter.subscribeChanges((topics) => {
        for (const topic of new Set(topics)) {
          for (const key of TOPIC_KEYS[topic]) void client.invalidateQueries({ queryKey: key });
        }
      }),
    [adapter, client],
  );
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
        <ChangeBridge adapter={adapter} />
        <GateParents adapter={adapter} />
        {children}
      </QueryClientProvider>
    </AdapterContext.Provider>
  );
}
