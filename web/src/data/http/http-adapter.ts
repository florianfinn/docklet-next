// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original HTTP and SSE adapter for issue 38.

import type { operations, paths } from "@/api/generated/schema";

import type { DockletAdapter, Topic } from "../adapter";
import { AdapterOutcomeError, GENERIC_REASON, HUB_REF, unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import { createDisconnectedAdapter } from "../disconnected-adapter";
import type { CodedActionRun } from "../message-code";
import { mapActivityDetail, mapEvent } from "./map-dto";

type ReadOperation = "get-capabilities" | "listEnvironments" | "list-activities" | "get-activity" | "listEvents";
type Body<I extends ReadOperation> = operations[I]["responses"][200]["content"]["application/json"];
type StreamPayload = operations["streamClient"]["responses"][200]["content"]["text/event-stream"][number]["data"];
type Source = Pick<EventSource, "close" | "onopen" | "onmessage" | "onerror"> & Partial<Pick<EventSource, "readyState">>;

const IMPLEMENTED = new Set(["listActions", "listEvents", "subscribeActions", "subscribeChanges"]);
const RECONNECT_MS = 250;

export interface HttpAdapterOptions {
  baseUrl?: string;
  fetch?: typeof globalThis.fetch;
  eventSource?: (url: string) => Source;
}

function refFor(operationId: string, args: readonly unknown[]): ResourceRef {
  const id = typeof args[0] === "string" ? args[0] : "";
  if (!id) return HUB_REF;
  if (/Stack|Service/.test(operationId)) return { kind: "stack", id };
  if (/Container/.test(operationId)) return { kind: "container", id };
  if (/Volume|File|Backup/.test(operationId)) return { kind: "volume", id };
  if (/Host|Agent/.test(operationId)) return { kind: "host", id };
  return HUB_REF;
}

function classifiedError(error: unknown, status?: number): AdapterOutcomeError {
  if (error instanceof AdapterOutcomeError) return error;
  const object = error && typeof error === "object" ? error as Record<string, unknown> : {};
  if ((object.code === "run.busy" || object.code === "run.cancelling") && typeof object.activeRunId === "string" && object.activeRunId) {
    return new AdapterOutcomeError({ kind: "conflict", code: object.code, activeRunId: object.activeRunId });
  }
  return new AdapterOutcomeError({ kind: "error", code: typeof object.code === "string" ? object.code : "http.error", status });
}

/** A small typed caller; response shape is checked before a DTO reaches the mapper. */
export function createHttpAdapter(options: HttpAdapterOptions = {}): DockletAdapter {
  const baseUrl = (options.baseUrl ?? "/api").replace(/\/$/, "");
  const fetcher = options.fetch ?? globalThis.fetch;
  const sourceFactory = options.eventSource ?? ((url: string) => new EventSource(url, { withCredentials: true }));
  const changeListeners = new Set<(topics: Topic[]) => void>();
  const actionListeners = new Set<(runs: CodedActionRun[]) => void>();
  let source: Source | undefined;
  let currentRuns: CodedActionRun[] | undefined;
  let lastEventId: number | undefined;
  let subscriptionVersion = 0;
  let readSequence = 0;
  let lastFullSequence = 0;
  // Reads apply per run only when they started after its last applied read.
  // Full reads record removals, so older results cannot restore absent runs.
  const appliedRunSequences = new Map<string, number>();
  let sourceOpen = false;
  let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

  async function read<I extends ReadOperation>(operationId: I, route: keyof paths | string, query?: Record<string, string | number>): Promise<Body<I>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query ?? {})) params.set(key, String(value));
    const url = `${baseUrl}${route}${params.size ? `?${params}` : ""}`;
    let response: Response;
    try {
      response = await fetcher(url, { credentials: "same-origin", headers: { Accept: "application/json" } });
    } catch (error) { throw classifiedError(error); }
    let body: unknown;
    try { body = await response.json(); } catch (error) { throw classifiedError(error, response.status); }
    if (!response.ok) throw classifiedError(body, response.status);
    if (!body || typeof body !== "object" || (body as { success?: unknown }).success !== true || !("data" in body)) {
      throw classifiedError({ code: `${operationId}.invalid-response` }, response.status);
    }
    return body as Body<I>;
  }

  async function getCapability(operationId: string, resourceRef: ResourceRef = HUB_REF): Promise<CapabilityDescriptor> {
    const fallback = (reasonCode = GENERIC_REASON): CapabilityDescriptor => ({ operationId, resourceRef, available: false, disabledReasonCode: reasonCode });
    let descriptors: Body<"get-capabilities">["data"];
    try {
      const body = await read("get-capabilities", "/capabilities", { kind: resourceRef.kind, id: resourceRef.id });
      descriptors = body.data;
    } catch { return fallback(); }
    if (!Array.isArray(descriptors)) return fallback();
    const found = descriptors.find((entry) => entry?.operationId === operationId && entry.resourceRef?.kind === resourceRef.kind && entry.resourceRef.id === resourceRef.id);
    if (!found || typeof found.available !== "boolean") return fallback();
    if (!found.available) return fallback(found.disabledReasonCode || GENERIC_REASON);
    if (!IMPLEMENTED.has(operationId)) return fallback("capability.mapping-unavailable");
    return { operationId, resourceRef, available: true };
  }

  async function requireCapability(operationId: string): Promise<void> {
    const descriptor = await getCapability(operationId);
    if (!descriptor.available) throw unavailable(operationId, descriptor.resourceRef, descriptor.disabledReasonCode);
  }

  async function page<I extends "listEnvironments" | "list-activities" | "listEvents">(operationId: I, route: string, limit: number): Promise<NonNullable<Body<I>["data"]>> {
    const result: NonNullable<Body<I>["data"]> = [] as unknown as NonNullable<Body<I>["data"]>;
    let start = 0;
    for (;;) {
      const body = await read(operationId, route, { start, limit, order: "desc" });
      if (!Array.isArray(body.data) || !body.pagination || !Number.isInteger(body.pagination.totalItems)) throw classifiedError({ code: `${operationId}.invalid-page` });
      result.push(...body.data as never[]);
      start += body.data.length;
      if (start >= body.pagination.totalItems) return result;
      if (body.data.length === 0) throw classifiedError({ code: `${operationId}.short-page` });
    }
  }

  async function loadActions(): Promise<CodedActionRun[]> {
    await requireCapability("listActions");
    const environments = await page("listEnvironments", "/environments", 100);
    const ids = new Set(["0", ...environments.map((item) => item.id)]);
    const batches = await Promise.all([...ids].map((id) => page("list-activities", `/environments/${encodeURIComponent(id)}/activities`, 100)));
    const items = batches.flat();
    const runs = await Promise.all(items.map(async (item) => {
      const body = await read("get-activity", `/environments/${encodeURIComponent(item.environmentId)}/activities/${encodeURIComponent(item.id)}`, { limit: 500 });
      if (!body.data || !body.data.activity || body.data.activity.id !== item.id) throw classifiedError({ code: "get-activity.invalid-detail" });
      try { return mapActivityDetail(body.data); } catch (error) { throw classifiedError({ code: "get-activity.mapping-error", cause: error }); }
    }));
    return runs.sort((a, b) => Date.parse(b.startedAt) - Date.parse(a.startedAt));
  }

  async function loadEvents() {
    await requireCapability("listEvents");
    const events = await page("listEvents", "/events", 500);
    try { return events.map(mapEvent); } catch (error) { throw classifiedError({ code: "listEvents.mapping-error", cause: error }); }
  }

  function refreshRuns(): void {
    if (actionListeners.size === 0) return;
    const sequence = ++readSequence;
    const generation = subscriptionVersion;
    void loadActions().then((runs) => {
      if (generation !== subscriptionVersion || !actionListeners.size) return;
      const merged = new Map(currentRuns?.map((run) => [run.id, run]));
      const incoming = new Set(runs.map((run) => run.id));
      const canApply = (id: string) => sequence > Math.max(appliedRunSequences.get(id) ?? 0, lastFullSequence);
      for (const run of runs) {
        if (!canApply(run.id)) continue;
        merged.set(run.id, run);
        appliedRunSequences.set(run.id, sequence);
      }
      for (const id of merged.keys()) {
        if (incoming.has(id) || !canApply(id)) continue;
        merged.delete(id);
        appliedRunSequences.set(id, sequence);
      }
      lastFullSequence = Math.max(lastFullSequence, sequence);
      currentRuns = [...merged.values()].sort((a, b) => Date.parse(b.startedAt) - Date.parse(a.startedAt));
      actionListeners.forEach((listener) => listener(currentRuns!));
    }).catch((error) => { window.dispatchEvent(new CustomEvent("docklet:adapter-error", { detail: error })); });
  }

  function refreshRun(activityId: string, environmentId?: string): void {
    const previous = currentRuns?.find((run) => run.id === activityId);
    if (!previous || !environmentId || previous.hostId !== environmentId) { refreshRuns(); return; }
    const generation = subscriptionVersion;
    const sequence = ++readSequence;
    void read("get-activity", `/environments/${encodeURIComponent(environmentId)}/activities/${encodeURIComponent(activityId)}`, { limit: 500 }).then((body) => {
      if (!body.data || body.data.activity?.id !== activityId) throw classifiedError({ code: "get-activity.invalid-detail" });
      const updated = mapActivityDetail(body.data);
      if (generation !== subscriptionVersion || sequence <= Math.max(appliedRunSequences.get(activityId) ?? 0, lastFullSequence) || !currentRuns?.some((run) => run.id === activityId)) return;
      appliedRunSequences.set(activityId, sequence);
      currentRuns = currentRuns.map((run) => run.id === activityId ? updated : run);
      actionListeners.forEach((listener) => listener(currentRuns!));
      if (updated.status !== "queued" && updated.status !== "running") refreshRuns();
    }).catch((error) => { window.dispatchEvent(new CustomEvent("docklet:adapter-error", { detail: classifiedError(error) })); });
  }

  function refreshSubscribed(): void {
    refreshRuns();
    if (changeListeners.size) changeListeners.forEach((listener) => listener(["events"]));
  }

  function handleStream(data: string, eventId: string): void {
    const id = Number(eventId);
    if (eventId && Number.isInteger(id)) {
      if (lastEventId !== undefined && id !== lastEventId + 1) refreshSubscribed();
      lastEventId = id;
    }
    let envelope: StreamPayload;
    try { envelope = JSON.parse(data) as StreamPayload; } catch { refreshSubscribed(); return; }
    if (!envelope || typeof envelope !== "object" || typeof envelope.timestamp !== "string") { refreshSubscribed(); return; }
    if (envelope.type === "heartbeat") return;
    if (envelope.channel === "activities" && envelope.activity && typeof envelope.activity.type === "string") {
      const activityId = envelope.activity.activityId ?? envelope.activity.activity?.id;
      if (activityId) refreshRun(activityId, envelope.activity.environmentId ?? envelope.activity.activity?.environmentId);
      else refreshRuns();
      return;
    }
    if (envelope.channel === "events" && envelope.eventLog && typeof envelope.eventLog.type === "string") {
      changeListeners.forEach((listener) => listener(["events"]));
      return;
    }
    refreshSubscribed();
  }

  function connect(): void {
    if (source || (!actionListeners.size && !changeListeners.size)) return;
    sourceOpen = false;
    let opened: Source;
    try {
      opened = sourceFactory(`${baseUrl}/stream?channels=activities%2Cevents`);
    } catch (error) {
      window.dispatchEvent(new CustomEvent("docklet:adapter-error", { detail: classifiedError(error) }));
      scheduleReconnect();
      return;
    }
    source = opened;
    opened.onopen = () => {
      if (source !== opened) return;
      sourceOpen = true;
      lastEventId = undefined;
      refreshSubscribed();
    };
    opened.onmessage = (event: MessageEvent) => { if (source === opened) handleStream(event.data, event.lastEventId); };
    opened.onerror = () => {
      if (source !== opened) return;
      lastEventId = undefined;
      if (opened.readyState === 2) {
        opened.close();
        source = undefined;
        sourceOpen = false;
        scheduleReconnect();
      }
    };
  }

  function scheduleReconnect(): void {
    if (reconnectTimer || (!actionListeners.size && !changeListeners.size)) return;
    reconnectTimer = setTimeout(() => {
      reconnectTimer = undefined;
      connect();
    }, RECONNECT_MS);
  }

  function disconnectIfUnused(): void {
    if (actionListeners.size || changeListeners.size) return;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    reconnectTimer = undefined;
    source?.close();
    source = undefined;
    sourceOpen = false;
    ++subscriptionVersion;
    lastFullSequence = 0;
    appliedRunSequences.clear();
  }

  const unsupported = createDisconnectedAdapter();
  const methods = Object.fromEntries(Object.keys(unsupported).filter((key) => key !== "kind").map((operationId) => [operationId, (...args: unknown[]) => Promise.reject(unavailable(operationId, refFor(operationId, args), "capability.mapping-unavailable"))])) as unknown as DockletAdapter;
  return {
    ...methods,
    kind: "http",
    mappedOperations: IMPLEMENTED,
    getCapability,
    listActions: loadActions,
    listEvents: loadEvents,
    subscribeActions(listener) {
      actionListeners.add(listener);
      connect();
      if (actionListeners.size === 1 && sourceOpen) refreshRuns();
      else if (currentRuns) listener(currentRuns);
      return () => { actionListeners.delete(listener); disconnectIfUnused(); };
    },
    subscribeChanges(listener) {
      changeListeners.add(listener);
      connect();
      return () => { changeListeners.delete(listener); disconnectIfUnused(); };
    },
    subscribeMetrics: () => () => undefined,
    subscribeLogs: () => () => undefined,
    subscribeContainerLogs: () => () => undefined,
  };
}
