// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/data/AdapterProvider.tsx
// Revision: 8dcaf91635d017da67d2ad497797780c07348ef569f884d278f117e3dce3f929
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: #69 connected identity and hub settings. Issue #77: session-bound closure and identity generation cleanup. R1: preserve the confirmed currentUser during active identity replacement and establishment. Production image/registry UI and validation for #230. M12 #284 live volume backup domain, transport, UI and regression coverage. M14 #314: invalidate capability queries with container, stack and host changes. M18 #341 #343 #342 #344 #250 #346 #319: v14 policies, explicit update/recovery gestures and shared scheduling rule. M20 #360: web hardening step (#359 #24 #175 #177 #383 #421 #271). M23 #391: app query client with the hub maintenance retry policy (app-query-client.ts). M23 #435: bounded Dashboard events window and timeline slots. M26 #462: bounded shell counts and raw inventory ownership.

import { QueryClientProvider, useQueryClient } from "@tanstack/react-query";
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
import { createAppQueryClient } from "./app-query-client";
import { refresh } from "./refresh-owner";

const TOPIC_KEYS: Record<Topic, readonly (readonly unknown[])[]> = {
  hosts: [queryKeys.hosts, queryKeys.systemComponents],
  // Images derive their consumers from containers, so container and stack changes refresh them too.
  stacks: [queryKeys.stacks, queryKeys.stackRoot, queryKeys.images, queryKeys.navInventory],
  containers: [queryKeys.containers, queryKeys.images, queryKeys.navInventory, ["container"]],
  security: [queryKeys.security],
  topology: [queryKeys.topology, queryKeys.networks],
  events: [queryKeys.events],
  dashboardEvents: [queryKeys.dashboardEvents],
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
      const offRecent = registerReadConsumer(adapter.subscribeChanges, "dashboardEvents", () => client.getQueryCache().find({ queryKey: queryKeys.dashboardEvents, exact: true })?.isActive() ?? false);
      const offStacks = adapter.subscribeStackTerminals?.(({ stackId, hostId }) => {
        // The stack's own descriptors and its host's container descriptors may change with the run.
        const capabilities = client.getQueryCache().findAll({ predicate: ({ queryKey: [root, , kind, id] }) => root === "capability"
          && (kind === "stack" && id === stackId || kind === "container" && typeof id === "string" && id.startsWith(`${hostId}~`)) });
        refresh(client, [queryKeys.stack(stackId)], [...capabilities.map((query) => query.queryKey), queryKeys.stacks, queryKeys.containers, queryKeys.images, queryKeys.volumes, queryKeys.topology, queryKeys.networks]);
      });
      // One notification refreshes each matching query once and never aborts a read in flight (refresh-owner.ts).
      const offChanges = adapter.subscribeChanges((topics) => {
        const capability = topics.some((topic) => topic === "containers" || topic === "stacks" || topic === "hosts") ? [["capability"]] : [];
        refresh(client, [...capability, ...[...new Set(topics)].flatMap((topic) => TOPIC_KEYS[topic])]);
      });
      return () => { offDemand(); offRecent(); offChanges(); offStacks?.(); };
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
  const [client] = useState(createAppQueryClient);
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
