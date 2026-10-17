import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";
import { useLocale } from "use-intl";

import type { AppLocale } from "@/domain/locale";

import { localizeContainers, localizeSchedule, localizeSecurity } from "@/runtime";
import { renderEvent, renderStackDetail, type CodedEvent, type CodedStackDetail } from "./message-code";
import { useAdapter } from "./use-adapter";
import { queryKeys } from "./query-keys";

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
  return useQuery({ queryKey: queryKeys.stacks, queryFn: ({ signal }) => adapter.listStacks((rows) => { if (!signal.aborted) client.setQueryData(queryKeys.stacks, rows); }) });
}

export function useStack(id: string | undefined) {
  const adapter = useAdapter();
  const locale = useLocale();
  const select = useMemo(() => (stack: CodedStackDetail) => renderStackDetail(stack, locale), [locale]);
  return useQuery({ queryKey: queryKeys.stack(id ?? ""), queryFn: () => adapter.getStack(id ?? ""), enabled: !!id, select });
}

export function useContainers() {
  const adapter = useAdapter();
  const client = useQueryClient();
  return useQuery({ queryKey: queryKeys.containers, queryFn: ({ signal }) => adapter.listContainers((rows) => { if (!signal.aborted) client.setQueryData(queryKeys.containers, rows); }), select: useLocalized(localizeContainers) });
}

export function useSecurity() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.security, queryFn: () => adapter.getSecurity(), select: useLocalized(localizeSecurity) });
}

export function useTopology() {
  const adapter = useAdapter();
  const client = useQueryClient();
  return useQuery({ queryKey: queryKeys.topology, queryFn: ({ signal }) => adapter.getTopology((rows) => { if (!signal.aborted) client.setQueryData(queryKeys.topology, rows); }) });
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

export function useSchedule() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.schedule, queryFn: () => adapter.listSchedule(), select: useLocalized(localizeSchedule) });
}

export function useVolumes() {
  const adapter = useAdapter();
  const client = useQueryClient();
  return useQuery({ queryKey: queryKeys.volumes, queryFn: ({ signal }) => adapter.listVolumes((rows) => { if (!signal.aborted) client.setQueryData(queryKeys.volumes, rows); }) });
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
  return useQuery({ queryKey: queryKeys.currentUser, queryFn: () => adapter.getCurrentUser(), enabled, staleTime: 0 });
}

export function useSetupStatus(enabled = true) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.setupStatus, queryFn: () => adapter.getSetupStatus(), enabled, staleTime: 0 });
}

export function useImages() {
  const adapter = useAdapter();
  const client = useQueryClient();
  return useQuery({ queryKey: queryKeys.images, queryFn: ({ signal }) => adapter.listImages((rows) => { if (!signal.aborted) client.setQueryData(queryKeys.images, rows); }) });
}
