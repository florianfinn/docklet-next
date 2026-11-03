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

export type ConditionReader = (hostId: string) => Promise<ConditionsByContainer>;

/** Reads the container conditions of one host keyed by Docker id; throws when the read fails. */
async function readHost(baseUrl: string, fetcher: typeof fetch, hostId: string): Promise<ConditionsByContainer> {
  const response = await fetcher(`${baseUrl}/environments/${encodeURIComponent(hostId)}/runtime-conditions`, { credentials: "same-origin", headers: { Accept: "application/json" } });
  if (!response.ok) { void response.body?.cancel(); throw new Error("conditions.unavailable"); }
  const body = await response.json() as { success?: unknown; data?: { items?: WireCondition[] | null } };
  if (body?.success !== true || !body.data || (body.data.items != null && !Array.isArray(body.data.items))) throw new Error("conditions.invalid");
  return new Map((body.data.items ?? []).flatMap((item) => item?.kind === "container" && item.containerId && item.condition !== "nodata"
    ? [[item.containerId, { condition: item.condition, reason: item.reason, ...(item.exitCode ? { exitCode: item.exitCode } : {}), ...(item.restarts10m ? { restarts10m: item.restarts10m } : {}), sampledAt: item.sampledAt }] as const]
    : []));
}

/**
 * Condition reads per host for one signed-in identity. A failed read keeps the last good
 * answer of that host, so a known crash stays red; without one, exited containers stay grey.
 * `reset` forgets every host when the identity changes.
 */
export function createConditionReader(baseUrl: string, fetcher: typeof fetch): { read: ConditionReader; reset: () => void } {
  let last = new Map<string, ConditionsByContainer>();
  return {
    async read(hostId) {
      const owner = last;
      try {
        const conditions = await readHost(baseUrl, fetcher, hostId);
        if (owner === last) last.set(hostId, conditions);
        return conditions;
      } catch {
        return owner.get(hostId) ?? new Map();
      }
    },
    reset() { last = new Map(); },
  };
}

/** One-off read without retained answers. */
export function readRuntimeConditions(baseUrl: string, fetcher: typeof fetch, hostId: string): Promise<ConditionsByContainer> {
  return createConditionReader(baseUrl, fetcher).read(hostId);
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
      // Services carry the host-qualified id like containers; conditions are keyed by the Docker id.
      const found = service.containerId ? conditions.get(splitInventoryId(service.containerId)[1]) : undefined;
      return found ? { ...service, condition: { ...found, containerId: service.containerId! } } : service;
    }),
  }));
  return { ...batch, stacks, containers };
}
