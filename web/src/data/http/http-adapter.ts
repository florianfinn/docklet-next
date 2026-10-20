// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original HTTP and SSE adapter for issue 38.

import { CancelledError, isCancelledError, QueryClient } from "@tanstack/react-query";

import type { components, operations, paths } from "@/api/generated/schema";

import type { ActionReadHealth, DockletAdapter, Topic } from "../adapter";
import { AdapterOutcomeError, GENERIC_REASON, HUB_REF, unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import { createDisconnectedAdapter } from "../disconnected-adapter";
import type { CodedActionRun } from "../message-code";
import type { InventoryObservation } from "@/domain/model";
import { createMetricsReader, type MetricsSocket } from "./metrics";
import { createResourceReader } from "./resources";
import { createInventoryReader } from "./inventory";
import { mapHost } from "./hosts";
import { createHostCommands } from "./host-commands";
import { mapEvent } from "./map-dto";
import { createHttpHubBackups } from "./hub-backups";
import { createCsrfClient } from "./csrf-client";
import { createHttpAuth, type SessionChange } from "./auth";
import { createSettingsCore } from "./settings-core";
import { createSharedReads } from "./shared-reads";
import { runtimeTokenReads } from "./runtime-token-read";
import { createRuntimeCommands } from "./runtime-commands";
import { createRuntimeLogs } from "./logs";
import { registerIdentityLifetime } from "../identity-lifetime";
import { createIdentityReads } from "./identity-reads";
import { createActivityReader } from "./activity-reader";

type ReadOperation = "get-capabilities" | "listEnvironments" | "list-activities" | "get-activity" | "listEvents";
type Body<I extends ReadOperation> = operations[I]["responses"][200]["content"]["application/json"];
type StreamPayload = operations["streamClient"]["responses"][200]["content"]["text/event-stream"][number]["data"];
type Source = Pick<EventSource, "close" | "onopen" | "onmessage" | "onerror"> & Partial<Pick<EventSource, "readyState">>;

const IMPLEMENTED = new Set(["subscribeLogs", "subscribeContainerLogs", "runStackCommand", "runContainerCommand", "registerHost", "issueAgentToken", "removeHost", "discoverHub", "listStacks", "getStack", "listContainers", "listHosts", "listActions", "listEvents", "subscribeActions", "subscribeChanges", "getAuthMode", "getCurrentUser", "login", "logout", "changePassword", "updateMyLocale", "getSetupStatus", "completeSetup", "getConnectedSettings", "saveSettings", "listImages", "listVolumes", "getVolume", "getTopology", "subscribeMetrics"]);
const RECONNECT_MS = 250;

export interface HttpAdapterOptions {
  baseUrl?: string;
  fetch?: typeof globalThis.fetch;
  eventSource?: (url: string) => Source;
  webSocket?: (url: string) => MetricsSocket;
  uploadFactory?: () => XMLHttpRequest;
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
  const envelope = error && typeof error === "object" ? error as Record<string, unknown> : {};
  const object = envelope.data && typeof envelope.data === "object" ? envelope.data as Record<string, unknown> : envelope;
  if (status === 409 && object.code === "run.request-mismatch") {
    return new AdapterOutcomeError({ kind: "conflict", code: "run.request-mismatch" });
  }
  if ((object.code === "run.busy" || object.code === "run.cancelling") && typeof object.activeRunId === "string" && object.activeRunId) {
    return new AdapterOutcomeError({ kind: "conflict", code: object.code, activeRunId: object.activeRunId });
  }
  return new AdapterOutcomeError({ kind: "error", code: typeof object.code === "string" ? object.code : "http.error", status });
}

/** A small typed caller; response shape is checked before a DTO reaches the mapper. */
export function createHttpAdapter(options: HttpAdapterOptions = {}): DockletAdapter {
  const baseUrl = (options.baseUrl ?? "/api").replace(/\/$/, "");
  const identityReads = createIdentityReads(options.fetch ?? globalThis.fetch);
 const rawClient = createCsrfClient(baseUrl, runtimeTokenReads(baseUrl, identityReads.fetch));
 const client = { token: rawClient.token, fetch: (async (input, init) => {
   const version = identityGeneration;
   const response = await rawClient.fetch(input, init);
   const path = new URL(String(input), window.location.href).pathname;
   if (version === identityGeneration && response.status === 401 && !["/auth/login", "/auth/password", "/auth/me"].some((suffix) => path.endsWith(suffix)) && !/\/(activities|runtime-runs)(\/|$)/.test(path)) {
     const body = await response.arrayBuffer();
     if (version === identityGeneration) suspendIdentity();
     return new Response(body, { status: response.status, headers: response.headers });
   }
   return response;
 }) as typeof fetch };
  const sharedReads = createSharedReads(client.fetch);
  const fetcher = sharedReads.fetch;
  const sourceFactory = options.eventSource ?? ((url: string) => new EventSource(url, { withCredentials: true }));
  const changeListeners = new Set<(topics: Topic[]) => void>();
  const actionListeners = new Set<(runs: CodedActionRun[]) => void>();
  let source: Source | undefined;
  const healthListeners = new Set<(health: ActionReadHealth) => void>();
  let lastEventId: number | undefined;
  let sourceOpen = false;
  let authenticated = true;
  let identityGeneration = 0;
 let lifetimeGeneration = 0;
 const identityListeners = new Set<() => void>();
 let identityCheck: Promise<boolean> | undefined;
  let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

  async function read<I extends ReadOperation>(operationId: I, route: keyof paths | string, query?: Record<string, string | number>, observeResponse?: (response: Response) => void): Promise<Body<I>> {
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
    observeResponse?.(response);
    return body as Body<I>;
  }

  const capabilityReads = new Map<string, Promise<Body<"get-capabilities">["data"]>>();
  let environmentState: string | undefined;
  const inventoryTopics: Topic[] = ["stacks", "containers", "images", "volumes", "topology"];
  const environmentCache = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 10_000, gcTime: 30_000 } } });
  const environmentsKey = ["environments"];
  const environments = () => environmentCache.fetchQuery({ queryKey: environmentsKey, queryFn: () => page("listEnvironments", "/environments", 100) });
  function invalidateCapabilities() { capabilityReads.clear(); }
  function invalidateEnvironments() { sharedReads.clear(); void environmentCache.invalidateQueries({ queryKey: environmentsKey }); }
  function suspendIdentity(retain = false, nextActive = false) {
    ++identityGeneration;
    authenticated = nextActive;
 identityCheck = undefined;
    runtime.reset();
    logs.reset();
 metrics.reset(false, retain);
 identityReads.reset();
    sharedReads.clear(); invalidateCapabilities(); environmentCache.clear(); observations.clear();
    source?.close(); source = undefined; sourceOpen = false;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    reconnectTimer = undefined;
 if (!retain) { actions.reset(true); ++lifetimeGeneration; identityListeners.forEach((notify) => notify()); }
  }
  function invalidateIdentity(change: SessionChange) {
    if (change === "confirmed" && authenticated) return;
    const retain = change === "renewed" && authenticated;
    suspendIdentity(retain, change !== "lost");
    if (retain) actions.invalidate(true); else actions.reset(change === "lost");
    authenticated = change !== "lost";
    if (authenticated) { metrics.reset(true, retain); connect(); }
  }
  function environmentKey(rows: components["schemas"]["EnvironmentEnvironment"][]) {
    return JSON.stringify(rows.map((row) => [row.id, row.status, row.enabled, row.apiUrl, row.isEdge, row.agentReport?.version.currentVersion, row.agentReport?.version.agentContractVersion, row.agentReport?.version.agentContractFingerprint]).sort((a, b) => String(a[0]).localeCompare(String(b[0]))));
  }

  async function getCapability(operationId: string, resourceRef: ResourceRef = HUB_REF): Promise<CapabilityDescriptor> {
    const fallback = (reasonCode = GENERIC_REASON): CapabilityDescriptor => ({ operationId, resourceRef, available: false, disabledReasonCode: reasonCode });
    let descriptors: Body<"get-capabilities">["data"];
    try {
      const key = JSON.stringify([resourceRef.kind, resourceRef.id]);
      let pending = capabilityReads.get(key);
      if (!pending) {
        pending = read("get-capabilities", "/capabilities", { kind: resourceRef.kind, id: resourceRef.id }).then((body) => {
          if (!Array.isArray(body.data)) throw new Error("capabilities.invalid-descriptors");
          return body.data;
        });
        capabilityReads.set(key, pending);
        const started = pending;
        void pending.catch(() => { if (capabilityReads.get(key) === started) capabilityReads.delete(key); });
      }
      descriptors = await pending;
    } catch (error) {
      if (operationId === "listActions" && error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status === 401) throw error;
      return fallback();
    }
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

  async function page<I extends "listEnvironments" | "list-activities" | "listEvents">(operationId: I, route: string, limit: number, observeResponse?: (response: Response) => void): Promise<NonNullable<Body<I>["data"]>> {
    const result: NonNullable<Body<I>["data"]> = [] as unknown as NonNullable<Body<I>["data"]>;
    let start = 0;
    for (;;) {
      const body = await read(operationId, route, { start, limit, order: "desc" }, observeResponse);
      if (!Array.isArray(body.data) || !body.pagination || !Number.isInteger(body.pagination.totalItems)) throw classifiedError({ code: `${operationId}.invalid-page` });
      result.push(...body.data as never[]);
      start += body.data.length;
      if (start >= body.pagination.totalItems) return result;
      if (body.data.length === 0) throw classifiedError({ code: `${operationId}.short-page` });
    }
  }

  async function checkIdentity(): Promise<boolean> {
    if (identityCheck) return identityCheck;
    const version = identityGeneration;
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 10_000);
    const pending = (async () => {
      try {
        const response = await client.fetch(`${baseUrl}/auth/me`, { signal: controller.signal, credentials: "same-origin" });
        await response.arrayBuffer();
 if (version !== identityGeneration) return false;
        if (response.status === 401) { suspendIdentity(); return false; }
        if (!response.ok) throw classifiedError(null, response.status);
        return authenticated;
      } finally { clearTimeout(timeout); }
    })();
    identityCheck = pending;
    void pending.finally(() => { if (identityCheck === pending) identityCheck = undefined; }).catch(() => undefined);
    return pending;
  }
  const metrics = createMetricsReader(baseUrl, fetcher, listHosts, requireCapability, options.webSocket ?? ((url) => new WebSocket(url)), observeHost, checkIdentity);
  const logs = createRuntimeLogs({
    baseUrl, socket: options.webSocket ?? ((url) => new WebSocket(url)), capability: getCapability,
    authenticated: () => authenticated,
    checkIdentity: async (signal) => {
      const version = identityGeneration;
      const response = await client.fetch(`${baseUrl}/auth/me`, { signal, credentials: "same-origin" });
      await response.arrayBuffer();
 if (version !== identityGeneration) return false;
      if (response.status === 401) { suspendIdentity(); return false; }
      if (!response.ok) throw classifiedError(null, response.status);
      return authenticated;
    },
  });
  const runtime = createRuntimeCommands({ baseUrl, client, capability: getCapability, unauthorized: () => { suspendIdentity(); actions.reset(true); }, publish: (run) => actions.upsert(run) });
  const actions = createActivityReader({
    retained: runtime.retained,
    project: runtime.project,
    hosts: async () => {
      await requireCapability("listActions");
      return [...new Set(["0", ...(await environments()).map((item) => item.id)])];
    },
    page: async (hostId) => {
      let incomplete = false;
      const items = await page("list-activities", `/environments/${encodeURIComponent(hostId)}/activities`, 100, (response) => {
        if (response.headers.get("X-Docklet-Host-Reason") === "agent.unavailable") incomplete = true;
      });
      return { items, incomplete };
    },
    unauthorized: suspendIdentity,
    detail: async (hostId, id) => (await read("get-activity", `/environments/${encodeURIComponent(hostId)}/activities/${encodeURIComponent(id)}`, { limit: 500 })).data!,
    publish: (runs, health) => {
      healthListeners.forEach((listener) => listener(health));
      if (runs) actionListeners.forEach((listener) => listener(runs));
    },
  });

  async function loadEvents() {
    await requireCapability("listEvents");
    const events = await page("listEvents", "/events", 500);
    try { return events.map(mapEvent); } catch (error) { throw classifiedError({ code: "listEvents.mapping-error", cause: error }); }
  }

  function refreshRuns(): void {
    if (actionListeners.size === 0) return;
    void actions.load().catch((error) => { if (isCancelledError(error)) return; window.dispatchEvent(new CustomEvent("docklet:adapter-error", { detail: error })); });
  }

  function refreshRun(activityId: string, environmentId?: string): void {
    if (environmentId && runtime.retained().some((run) => run.id === activityId && run.hostId === environmentId)) {
      void runtime.detail(environmentId, activityId).catch(() => undefined);
      return;
    }
    const previous = actions.runs().find((run) => run.id === activityId && run.hostId === environmentId);
    if (!previous || !environmentId) { refreshRuns(); return; }
    void actions.refreshOne(activityId, environmentId).catch((error) => { if (!isCancelledError(error)) window.dispatchEvent(new CustomEvent("docklet:adapter-error", { detail: error })); });
  }

  function refreshSubscribed(): void {
    invalidateEnvironments();
    refreshRuns();
    if (changeListeners.size) changeListeners.forEach((listener) => listener(["events", "hosts", "stacks", "containers", "images", "volumes", "topology"]));
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
    // Post-notification reads must not join snapshots started before the change.
    sharedReads.clear();
    if (envelope.channel === "activities" && envelope.activity && typeof envelope.activity.type === "string") {
      const activityId = envelope.activity.activityId ?? envelope.activity.activity?.id;
      if (activityId) refreshRun(activityId, envelope.activity.environmentId ?? envelope.activity.activity?.environmentId);
      else refreshRuns();
      changeListeners.forEach((listener) => listener(["events"]));
      return;
    }
    if (envelope.channel === "events" && envelope.eventLog && typeof envelope.eventLog.type === "string") {
      changeListeners.forEach((listener) => listener(["events"]));
      return;
    }
    if (envelope.channel === "environments") {
      invalidateCapabilities();
      invalidateEnvironments();
      const rows = envelope.environment?.environments;
      const next = Array.isArray(rows) ? environmentKey(rows) : undefined;
      const changed = next === undefined || next !== environmentState;
      environmentState = next;
      if (changed) refreshRuns();
      changeListeners.forEach((listener) => listener(changed ? ["hosts", ...inventoryTopics] : ["hosts"]));
      return;
    }
    refreshSubscribed();
  }

  function connect(): void {
    if (!authenticated || source || (!actionListeners.size && !changeListeners.size)) return;
    sourceOpen = false;
    let opened: Source;
    try {
      opened = sourceFactory(`${baseUrl}/stream?channels=activities%2Cevents%2Cenvironments`);
    } catch (error) {
      window.dispatchEvent(new CustomEvent("docklet:adapter-error", { detail: classifiedError(error) }));
      scheduleReconnect();
      return;
    }
    source = opened;
    const version = identityGeneration;
 opened.onopen = () => {
      if (source !== opened || version !== identityGeneration) return;
      sourceOpen = true;
      lastEventId = undefined;
      refreshSubscribed();
    };
    opened.onmessage = (event: MessageEvent) => { if (source === opened && version === identityGeneration) handleStream(event.data, event.lastEventId); };
    opened.onerror = () => {
      if (source !== opened || version !== identityGeneration) return;
      lastEventId = undefined;
      opened.close(); source = undefined; sourceOpen = false;
      void checkIdentity().then((valid) => { if (valid && version === identityGeneration) scheduleReconnect(); }, () => { if (version === identityGeneration) scheduleReconnect(); });
    };
  }

  function scheduleReconnect(): void {
    if (!authenticated || reconnectTimer || (!actionListeners.size && !changeListeners.size)) return;
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
    actions.invalidate();
    runtime.observe(false);
  }

  const observations = new Map<string, InventoryObservation>();
  async function listHosts() {
 const version = identityGeneration;
    await requireCapability("listHosts");
    const rows = await environments();
 if (version !== identityGeneration) throw new CancelledError();
    try {
      const hosts = rows.map((row) => ({ ...mapHost(row), inventory: observations.get(row.id) }));
      environmentState = environmentKey(rows);
      return hosts;
    }
    catch (error) { throw classifiedError({ code: "listHosts.mapping-error", cause: error }); }
  }
  function observeHost(hostId: string, state: InventoryObservation) {
    const old = observations.get(hostId);
    observations.set(hostId, state);
    if (JSON.stringify(old) !== JSON.stringify(state)) changeListeners.forEach((listener) => listener(["hosts"]));
  }
  function scopedObservations<T extends object>(make: (observe: typeof observeHost) => T): T {
    return new Proxy(make(() => {}), { get(_target, key) {
      return (...args: unknown[]) => {
        const version = identityGeneration;
        const reader = make((hostId, state) => { if (version === identityGeneration) observeHost(hostId, state); });
        const method = Reflect.get(reader, key) as (...values: unknown[]) => unknown;
        return method(...args);
      };
    } });
  }
  const inventory = scopedObservations((observe) => createInventoryReader(baseUrl, fetcher, listHosts, requireCapability, observe));
  const resourceReads = scopedObservations((observe) => createResourceReader(baseUrl, fetcher, listHosts, requireCapability, observe));
  const unsupported = createDisconnectedAdapter();
  const methods = Object.fromEntries(Object.keys(unsupported).filter((key) => key !== "kind").map((operationId) => [operationId, (...args: unknown[]) => Promise.reject(unavailable(operationId, refFor(operationId, args), "capability.mapping-unavailable"))])) as unknown as DockletAdapter;
  const adapter: DockletAdapter = {
    ...methods,
    ...createHttpAuth(baseUrl, client, invalidateIdentity, () => identityGeneration),
    ...createHostCommands(baseUrl, client, requireCapability, () => { invalidateCapabilities(); invalidateEnvironments(); changeListeners.forEach((listener) => listener(["hosts", ...inventoryTopics])); }),
    runStackCommand: runtime.runStackCommand,
    runContainerCommand: runtime.runContainerCommand,
    getRuntimeRun: runtime.getRuntimeRun,
    cancelRuntimeRun: runtime.cancelRuntimeRun,
    releaseRuntimeRun: runtime.releaseRuntimeRun,
    ...createSettingsCore(baseUrl, client),
    ...createHttpHubBackups(baseUrl, client, options.uploadFactory),
    kind: "http",
    mappedOperations: IMPLEMENTED,
    getCapability,
    listHosts,
    ...inventory,
    ...resourceReads,
    listActions: actions.load,
    subscribeActionReadHealth(listener) {
      healthListeners.add(listener);
      listener(actions.health());
      return () => { healthListeners.delete(listener); };
    },
    listEvents: loadEvents,
    subscribeActions(listener) {
      actionListeners.add(listener);
      runtime.observe(true);
      connect();
      if (actionListeners.size === 1 && sourceOpen) refreshRuns();
      else if (actions.hasSnapshot()) listener(actions.runs());
      return () => { actionListeners.delete(listener); disconnectIfUnused(); };
    },
    subscribeChanges(listener) {
      changeListeners.add(listener);
      connect();
      return () => { changeListeners.delete(listener); disconnectIfUnused(); };
    },
    subscribeMetrics: metrics,
    subscribeLogs: logs.subscribeLogs,
    subscribeContainerLogs: logs.subscribeContainerLogs,
  };
 registerIdentityLifetime(adapter.subscribeChanges, { version: () => lifetimeGeneration, active: () => authenticated, listen: (notify) => { identityListeners.add(notify); return () => { identityListeners.delete(notify); }; } });
 // Guard public reads and partial publishers in addition to transport bodies.
 return new Proxy(adapter, { get(target, key, receiver) {
  const method: unknown = Reflect.get(target, key, receiver);
  if (typeof method !== "function" || !/^(list|get|discover)/.test(String(key))) return method;
  return (...args: unknown[]) => {
    const version = identityGeneration;
    const check = () => { if (version !== identityGeneration) throw new CancelledError(); };
    const wrapped = args.map((arg) => typeof arg === "function" ? (...values: unknown[]) => { check(); return arg(...values); } : arg);
    // Auth owns its same-user renewal and initial establishment semantics.
    if (key === "getCurrentUser" || key === "getAuthMode") return method(...args);
    return Promise.resolve(method(...wrapped)).then((value) => { check(); return value; });
  };
 } });
}
