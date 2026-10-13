import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { useLocale } from "use-intl";

import type { AppLocale } from "@/domain/locale";

import { localizeContainers, localizeEvents, localizeSchedule, localizeSecurity } from "./demo/demo-text";
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
  return useQuery({ queryKey: queryKeys.stacks, queryFn: () => adapter.listStacks() });
}

export function useStack(id: string | undefined) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.stack(id ?? ""), queryFn: () => adapter.getStack(id ?? ""), enabled: !!id });
}

export function useContainers() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.containers, queryFn: () => adapter.listContainers(), select: useLocalized(localizeContainers) });
}

export function useSecurity() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.security, queryFn: () => adapter.getSecurity(), select: useLocalized(localizeSecurity) });
}

export function useTopology() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.topology, queryFn: () => adapter.getTopology() });
}

export function useEvents() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.events, queryFn: () => adapter.listEvents(), select: useLocalized(localizeEvents) });
}

export function useSchedule() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.schedule, queryFn: () => adapter.listSchedule(), select: useLocalized(localizeSchedule) });
}

export function useVolumes() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.volumes, queryFn: () => adapter.listVolumes() });
}

export function useVolume(id: string | undefined) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.volume(id ?? ""), queryFn: () => adapter.getVolume(id ?? ""), enabled: !!id });
}

export function useFiles(volumeId: string, path: string, enabled = true) {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.files(volumeId, path), queryFn: () => adapter.listFiles(volumeId, path), enabled, placeholderData: (prev, prevQuery) => (prevQuery?.queryKey[1] === volumeId ? prev : undefined) });
}

export function useSettings() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.settings, queryFn: () => adapter.getSettings() });
}

export function useImages() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.images, queryFn: () => adapter.listImages() });
}
