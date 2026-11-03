// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original live runtime condition merge for the fleet ring, issue #371.

import type { components } from "@/api/generated/schema";
import type { ContainerInfo, RuntimeCondition, Stack } from "@/domain/model";
import { splitInventoryId } from "./inventory-mapping";

type WireCondition = components["schemas"]["HistoryCondition"];
export type ConditionsByContainer = ReadonlyMap<string, Omit<RuntimeCondition, "containerId">>;

/**
 * Container conditions of one host keyed by Docker id. A failed read yields none: the ring then
 * shows exited containers grey, as without a matching condition.
 */
export async function readRuntimeConditions(baseUrl: string, fetcher: typeof fetch, hostId: string): Promise<ConditionsByContainer> {
  try {
    const response = await fetcher(`${baseUrl}/environments/${encodeURIComponent(hostId)}/runtime-conditions`, { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (!response.ok) { void response.body?.cancel(); return new Map(); }
    const body = await response.json() as { success?: unknown; data?: { items?: WireCondition[] | null } };
    const items = body?.success === true && Array.isArray(body.data?.items) ? body.data.items : [];
    return new Map(items.flatMap((item) => item?.kind === "container" && item.containerId && item.condition !== "nodata"
      ? [[item.containerId, { condition: item.condition, reason: item.reason, ...(item.exitCode ? { exitCode: item.exitCode } : {}), ...(item.restarts10m ? { restarts10m: item.restarts10m } : {}), sampledAt: item.sampledAt }] as const]
      : []));
  } catch {
    return new Map();
  }
}

/** Attaches each condition to the container and service that carry its Docker id, in their own id form. */
export function withConditions<T extends { stacks: Stack[]; containers: ContainerInfo[] }>(batch: T, conditions: ConditionsByContainer): T {
  if (!conditions.size) return batch;
  const containers = batch.containers.map((c) => {
    const found = conditions.get(splitInventoryId(c.id)[1]);
    return found ? { ...c, condition: { ...found, containerId: c.id } } : c;
  });
  const stacks = batch.stacks.map((stack) => ({
    ...stack,
    services: stack.services.map((service) => {
      const found = service.containerId ? conditions.get(service.containerId) : undefined;
      return found ? { ...service, condition: { ...found, containerId: service.containerId! } } : service;
    }),
  }));
  return { ...batch, stacks, containers };
}
