// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: M23 #389: automatic condition readers share the caller's read tag while retaining their identity-owned conditions. Original live runtime condition merge for the fleet ring, issue #371. M20 #360: web hardening step (#359 #24 #175 #177 #383 #421 #271). M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags.

import type { components } from "@/api/generated/schema";
import type { ContainerInfo, RuntimeCondition, Stack } from "@/domain/model";
import { splitInventoryId } from "./inventory-mapping";

type WireCondition = components["schemas"]["HistoryCondition"];
export type ConditionsByContainer = ReadonlyMap<string, Omit<RuntimeCondition, "containerId">>;

const KINDS: readonly unknown[] = ["container", "stack"];
const CONDITIONS: readonly unknown[] = ["running", "unhealthy", "down", "stopped", "nodata"];

const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);
const count = (value: unknown) => typeof value === "number" && Number.isInteger(value) && value >= 0;

function validRow(row: unknown): row is WireCondition {
  if (!isRecord(row) || !KINDS.includes(row.kind) || !CONDITIONS.includes(row.condition)) return false;
  if (row.kind === "stack") return true;
  return typeof row.containerId === "string" && row.containerId !== "" && typeof row.sampledAt === "string" && typeof row.reason === "string" && Number.isInteger(row.exitCode) && count(row.restarts10m);
}

/**
 * Decodes one host's condition snapshot keyed by Docker id. Only a success envelope whose data
 * object carries `items` as an array or null, with every row valid, is a snapshot; anything else
 * throws, so a malformed answer never replaces a retained one. Null or an empty list clears.
 */
export function decodeConditionSnapshot(body: unknown): ConditionsByContainer {
  if (!isRecord(body) || body.success !== true || !isRecord(body.data)) throw new Error("conditions.invalid");
  const items = body.data.items;
  // An absent list is malformed; only null or a list is an authoritative snapshot.
  if (items !== null && !Array.isArray(items)) throw new Error("conditions.invalid");
  const rows: unknown[] = items ?? [];
  if (!rows.every(validRow)) throw new Error("conditions.invalid");
  return new Map((rows as WireCondition[]).flatMap((item) => item.kind === "container" && item.condition !== "nodata"
    ? [[item.containerId, { condition: item.condition, reason: item.reason, ...(item.exitCode ? { exitCode: item.exitCode } : {}), ...(item.restarts10m ? { restarts10m: item.restarts10m } : {}), sampledAt: item.sampledAt }] as const]
    : []));
}

export type ConditionReader = (hostId: string, signal?: AbortSignal, fetcher?: typeof fetch) => Promise<ConditionsByContainer>;

/** Reads the container conditions of one host; throws when the read fails or the answer is malformed. */
async function readHost(baseUrl: string, fetcher: typeof fetch, hostId: string, signal?: AbortSignal): Promise<ConditionsByContainer> {
  const response = await fetcher(`${baseUrl}/environments/${encodeURIComponent(hostId)}/runtime-conditions`, { credentials: "same-origin", headers: { Accept: "application/json" }, signal });
  if (!response.ok) { void response.body?.cancel(); throw new Error("conditions.unavailable"); }
  return decodeConditionSnapshot(await response.json());
}

/**
 * Condition reads per host for one signed-in identity. A failed read keeps the last good
 * answer of that host, so a known crash stays red; without one, exited containers stay grey.
 * `reset` forgets every host when the identity changes.
 */
export function createConditionReader(baseUrl: string, fetcher: typeof fetch): { read: ConditionReader; reset: () => void } {
  let last = new Map<string, ConditionsByContainer>();
  return {
    async read(hostId, signal, source = fetcher) {
      const owner = last;
      try {
        const conditions = await readHost(baseUrl, source, hostId, signal);
        signal?.throwIfAborted();
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
export function readRuntimeConditions(baseUrl: string, fetcher: typeof fetch, hostId: string, signal?: AbortSignal): Promise<ConditionsByContainer> {
  return createConditionReader(baseUrl, fetcher).read(hostId, signal);
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
