// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/data/queries.ts
// Revision: 37ee7825503944c425b0597baaa8cd690a699acb460f1883e7e3cbc7b06d9c8b
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Adapter port cleanup, demo-only options and coded run/event localization after the historical import. See web/PROVENANCE.md. #38 HTTP adapter, live channel, capabilities, run states and fixed history. #69 connected first-run identity flow. Review #95: preserve cached rows during refetches and fall back to stack IDs when names are absent. Issue #77: session-bound closure and identity generation cleanup. M9 #204: preserve partial topology observations. M10 #231: production stack updates and lifecycle UI. Held-host security recovery for #263. Simple-user management, assigned-stack access and mode-aware self-service. M20 #360: web hardening step (#359 #24 #175 #177 #383 #421 #271). D4 #271: fair physical host reads and result-local recovery. M23 #425: shared start-up auth reads. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags.

import { keepPreviousData, useQuery, useQueryClient, type QueryClient, type QueryKey } from "@tanstack/react-query";
import { useMemo } from "react";
import { useLocale } from "use-intl";

import type { ContainerHistoryRef, HistoryRange } from "@/domain/history";
import type { ServiceKind } from "@/domain/service-widgets";
import type { AppLocale } from "@/domain/locale";
import { inventoryRecoveryInterval, inventoryStructuralSharing, INVENTORY_READ_TAG } from "./inventory-recovery";

import { localizeContainers, localizeSchedule, localizeSecurity } from "@/runtime";
import { renderEvent, renderStackDetail, type CodedEvent, type CodedStackDetail } from "./message-code";
import { useIsUser } from "./use-principal";
import { useAdapter } from "./use-adapter";
import type { RecentEvents } from "./adapter";
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

/** Each mounted inventory owns retries of its own omitted batches, independent of shared host observations. */
function useInventoryRecovery() {
  const hosts = useHosts();
  return { structuralSharing: inventoryStructuralSharing, refetchInterval: (query: { state: { data: unknown } }) => inventoryRecoveryInterval(query.state.data, hosts.data) };
}

export function useStacks(enabled = true) {
  const adapter = useAdapter();
  const recovery = useInventoryRecovery();
  const client = useQueryClient();
  return useQuery({ ...recovery, queryKey: queryKeys.stacks, enabled, queryFn: ({ signal }) => adapter.listStacks(initialBatches(client, queryKeys.stacks, signal), signal, INVENTORY_READ_TAG) });
}

export function useStack(id: string | undefined) {
  const adapter = useAdapter();
  const locale = useLocale();
  const select = useMemo(() => (stack: CodedStackDetail) => renderStackDetail(stack, locale), [locale]);
  return useQuery({ queryKey: queryKeys.stack(id ?? ""), queryFn: () => adapter.getStack(id ?? ""), enabled: !!id, select });
}

export function useContainers(enabled = true) {
  const user = useIsUser();
  const adapter = useAdapter();
  const recovery = useInventoryRecovery();
  const client = useQueryClient();
  return useQuery({ ...recovery, queryKey: queryKeys.containers, enabled: enabled && !user, queryFn: ({ signal }) => adapter.listContainers(initialBatches(client, queryKeys.containers, signal), signal, INVENTORY_READ_TAG), select: useLocalized(localizeContainers) });
}

export function useSecurity() {
  const adapter = useAdapter();
  const recovery = useInventoryRecovery();
  return useQuery({ ...recovery, queryKey: queryKeys.security, queryFn: ({ signal }) => adapter.getSecurity(signal), select: useLocalized(localizeSecurity) });
}

export function useTopology() {
  const adapter = useAdapter();
  const recovery = useInventoryRecovery();
  const client = useQueryClient();
  return useQuery({ ...recovery, queryKey: queryKeys.topology, queryFn: ({ signal }) => adapter.getTopology(initialBatches(client, queryKeys.topology, signal), client.getQueryData(queryKeys.topology), INVENTORY_READ_TAG) });
}

/** The Dashboard's security source: active counts per scanned image, without catalogue or findings reads. */
export function useActiveSecurity() {
  const adapter = useAdapter();
  const recovery = useInventoryRecovery();
  return useQuery({ ...recovery, queryKey: queryKeys.activeSecurity, queryFn: ({ signal }) => adapter.getActiveSecurity(signal) });
}

export function useEvents() {
  const adapter = useAdapter();
  const locale = useLocale();
  const select = useMemo(() => (events: CodedEvent[]) => events.map((event) => renderEvent(event, locale)), [locale]);
  return useQuery({ queryKey: queryKeys.events, queryFn: () => adapter.listEvents(), select });
}

/** The Dashboard's bounded window (24 h, at most 500); the Activity Center keeps the full history (`useEvents`). */
export function useDashboardEvents() {
  const adapter = useAdapter();
  const locale = useLocale();
  const select = useMemo(() => (page: RecentEvents) => ({ events: page.events.map((event) => renderEvent(event, locale)), totalItems: page.totalItems }), [locale]);
  return useQuery({ queryKey: queryKeys.dashboardEvents, queryFn: () => adapter.listDashboardEvents(), select });
}

export function useActionsQuery() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.actions, queryFn: () => adapter.listActions() });
}

export function useSchedule(hostIds?: readonly string[]) {
  const adapter = useAdapter();
  const key = [...queryKeys.schedule, ...(hostIds ? [[...hostIds].sort()] : [])];
  return useQuery({ queryKey: key, queryFn: () => adapter.listSchedule(hostIds, INVENTORY_READ_TAG), select: useLocalized(localizeSchedule) });
}

export function useVolumes() {
  const adapter = useAdapter();
  const recovery = useInventoryRecovery();
  const client = useQueryClient();
  return useQuery({ ...recovery, queryKey: queryKeys.volumes, queryFn: ({ signal }) => adapter.listVolumes(initialBatches(client, queryKeys.volumes, signal), signal, INVENTORY_READ_TAG) });
}

export function useVolume(id: string | undefined) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.volume(id ?? ""), queryFn: ({ signal }) => adapter.getVolume(id ?? "", signal), enabled: !!id });
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
  // One start-up read serves every mounting observer; explicit invalidations (login, renewal, identity events) still refetch.
  return useQuery({ queryKey: queryKeys.authMode, queryFn: () => adapter.getAuthMode(), staleTime: 60_000 });
}

export function useCurrentUser(enabled = true) {
  const adapter = useAdapter();
  return useQuery<Awaited<ReturnType<typeof adapter.getCurrentUser>> | null>({ queryKey: queryKeys.currentUser, queryFn: () => adapter.getCurrentUser(), enabled, staleTime: 60_000 });
}

export function useSetupStatus(enabled = true) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.setupStatus, queryFn: () => adapter.getSetupStatus(), enabled, staleTime: 0 });
}

export function useImages() {
  const adapter = useAdapter();
  const recovery = useInventoryRecovery();
  const client = useQueryClient();
  return useQuery({ ...recovery, queryKey: queryKeys.images, queryFn: ({ signal }) => adapter.listImages(initialBatches(client, queryKeys.images, signal), signal, INVENTORY_READ_TAG) });
}

/** History refreshes with the minute samples; a range switch keeps the previous bars until the new ones arrive. */
export function useContainerHistory(ref: ContainerHistoryRef | undefined, range: HistoryRange) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.containerHistory(ref?.hostId ?? "", ref?.name ?? "", range), enabled: !!ref?.name, queryFn: () => adapter.getContainerHistory(ref!, range), refetchInterval: 60_000, placeholderData: keepPreviousData });
}

export function useStackHistory(stackId: string | undefined, range: HistoryRange) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.stackHistory(stackId ?? "", range), enabled: !!stackId, queryFn: () => adapter.getStackHistory(stackId!, range), refetchInterval: 60_000, placeholderData: keepPreviousData });
}

export function useCapacity() {
  const adapter = useAdapter();
  const user = useIsUser();
  return useQuery({ queryKey: queryKeys.capacity, enabled: !user, queryFn: () => adapter.getCapacity(), refetchInterval: 15 * 60_000 });
}

/** Refresh matches the hub cache interval of each kind, so a poll never outruns it. */
const WIDGET_REFRESH: Record<ServiceKind, number> = { plex: 15_000, minecraft: 30_000, overseerr: 60_000 };

export function useServiceWidgets() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.serviceWidgets, queryFn: () => adapter.listServiceWidgets(), refetchInterval: 60_000 });
}

export function useServiceWidget(kind: ServiceKind, enabled = true) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.serviceWidget(kind), enabled, queryFn: () => adapter.getServiceWidget(kind), refetchInterval: WIDGET_REFRESH[kind] });
}

export function useServiceWidgetConfig(kind: ServiceKind) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.serviceWidgetConfig(kind), queryFn: () => adapter.getServiceWidgetConfig(kind) });
}

export function useNetworkSummaries() {
  const adapter = useAdapter();
  const recovery = useInventoryRecovery();
  return useQuery({ ...recovery, queryKey: queryKeys.networks, queryFn: ({ signal }) => adapter.listNetworkSummaries(signal, INVENTORY_READ_TAG) });
}
