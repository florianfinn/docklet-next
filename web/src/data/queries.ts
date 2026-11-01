// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/data/queries.ts
// Revision: 37ee7825503944c425b0597baaa8cd690a699acb460f1883e7e3cbc7b06d9c8b
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Adapter port cleanup, demo-only options and coded run/event localization after the historical import. See web/PROVENANCE.md. #38 HTTP adapter, live channel, capabilities, run states and fixed history. #69 connected first-run identity flow. Review #95: preserve cached rows during refetches and fall back to stack IDs when names are absent. Issue #77: session-bound closure and identity generation cleanup. M9 #204: preserve partial topology observations. M10 #231: production stack updates and lifecycle UI. Held-host security recovery for #263. Simple-user management, assigned-stack access and mode-aware self-service.

import { useQuery, useQueryClient, type QueryClient, type QueryKey } from "@tanstack/react-query";
import { useMemo } from "react";
import { useLocale } from "use-intl";

import type { AppLocale } from "@/domain/locale";
import { securityRecoveryInterval } from "@/domain/security";

import { localizeContainers, localizeSchedule, localizeSecurity } from "@/runtime";
import { renderEvent, renderStackDetail, type CodedEvent, type CodedStackDetail } from "./message-code";
import { useIsUser } from "./use-principal";
import { useAdapter } from "./use-adapter";
import { queryKeys } from "./query-keys";

// Initial reads stream host batches; refetches retain the complete cached result.
function initialBatches<T>(client: QueryClient, key: QueryKey, signal: AbortSignal): (rows: T) => void {
  const publish = client.getQueryData(key) === undefined;
  return (rows) => { if (publish && !signal.aborted) client.setQueryData(key, rows); };
}

/** Query select that shows demo texts in the interface language; German keeps the cached data as is. */
function useLocalized<T>(localize: (data: T, locale: AppLocale) => T): ((data: T) => T) | undefined {
  const locale = useLocale();
  return useMemo(() => (locale === "de" ? undefined : (data: T) => localize(data, locale)), [locale, localize]);
}

export function useHosts() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.hosts, queryFn: () => adapter.listHosts() });
}

export function useSystemComponents() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.systemComponents, queryFn: () => adapter.listSystemComponents() });
}

export function useStacks() {
  const adapter = useAdapter();
  const client = useQueryClient();
  return useQuery({ queryKey: queryKeys.stacks, queryFn: ({ signal }) => adapter.listStacks(initialBatches(client, queryKeys.stacks, signal)) });
}

export function useStack(id: string | undefined) {
  const adapter = useAdapter();
  const locale = useLocale();
  const select = useMemo(() => (stack: CodedStackDetail) => renderStackDetail(stack, locale), [locale]);
  return useQuery({ queryKey: queryKeys.stack(id ?? ""), queryFn: () => adapter.getStack(id ?? ""), enabled: !!id, select });
}

export function useContainers() {
  const user = useIsUser();
  const adapter = useAdapter();
  const client = useQueryClient();
  return useQuery({ queryKey: queryKeys.containers, enabled: !user, queryFn: ({ signal }) => adapter.listContainers(initialBatches(client, queryKeys.containers, signal)), select: useLocalized(localizeContainers) });
}

export function useSecurity() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.security, queryFn: () => adapter.getSecurity(), select: useLocalized(localizeSecurity), refetchInterval: (query) => securityRecoveryInterval(query.state.data) });
}

export function useTopology() {
  const adapter = useAdapter();
  const client = useQueryClient();
  return useQuery({ queryKey: queryKeys.topology, queryFn: ({ signal }) => adapter.getTopology(initialBatches(client, queryKeys.topology, signal), client.getQueryData(queryKeys.topology)) });
}

export function useEvents() {
  const adapter = useAdapter();
  const locale = useLocale();
  const select = useMemo(() => (events: CodedEvent[]) => events.map((event) => renderEvent(event, locale)), [locale]);
  return useQuery({ queryKey: queryKeys.events, queryFn: () => adapter.listEvents(), select });
}

export function useActionsQuery() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.actions, queryFn: () => adapter.listActions() });
}

export function useSchedule(hostIds?: readonly string[]) {
  const adapter = useAdapter();
  return useQuery({ queryKey: [...queryKeys.schedule, ...(hostIds ? [[...hostIds].sort()] : [])], queryFn: () => adapter.listSchedule(hostIds), select: useLocalized(localizeSchedule) });
}

export function useVolumes() {
  const adapter = useAdapter();
  const client = useQueryClient();
  return useQuery({ queryKey: queryKeys.volumes, queryFn: ({ signal }) => adapter.listVolumes(initialBatches(client, queryKeys.volumes, signal)) });
}

export function useVolume(id: string | undefined) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.volume(id ?? ""), queryFn: () => adapter.getVolume(id ?? ""), enabled: !!id });
}

export function useFiles(volumeId: string, path: string, enabled = true) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.files(volumeId, path), queryFn: () => adapter.listFiles(volumeId, path), enabled, placeholderData: (prev, prevQuery) => (prevQuery?.queryKey[1] === volumeId ? prev : undefined) });
}

export function useSettings(enabled = true) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.settings, queryFn: () => adapter.getSettings(), enabled });
}

export function useConnectedSettings(enabled = true) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.connectedSettings, queryFn: () => adapter.getConnectedSettings(), enabled });
}

export function useAuthMode() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.authMode, queryFn: () => adapter.getAuthMode(), staleTime: 0 });
}

export function useCurrentUser(enabled = true) {
  const adapter = useAdapter();
  return useQuery<Awaited<ReturnType<typeof adapter.getCurrentUser>> | null>({ queryKey: queryKeys.currentUser, queryFn: () => adapter.getCurrentUser(), enabled, staleTime: 0 });
}

export function useSetupStatus(enabled = true) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.setupStatus, queryFn: () => adapter.getSetupStatus(), enabled, staleTime: 0 });
}

export function useImages() {
  const adapter = useAdapter();
  const client = useQueryClient();
  return useQuery({ queryKey: queryKeys.images, queryFn: ({ signal }) => adapter.listImages(initialBatches(client, queryKeys.images, signal)) });
}
