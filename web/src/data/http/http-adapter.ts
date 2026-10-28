import { vulnerabilityTransport } from "./vulnerability-transport";
import { createVulnerabilities, readScanProgress } from "./vulnerabilities";
import { createStackUpdates } from "./stack-updates";
import { createImageCommands } from "./image-commands";
import { createRegistryCommands } from "./registry-commands";
// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original HTTP and SSE adapter for issue 38; notification-scoped activity/event refreshes and effective sequential event pages (#211).

import { hasReadConsumer } from "../read-consumers";
import { CancelledError, isCancelledError, QueryClient } from "@tanstack/react-query";

import type { components, operations, paths } from "@/api/generated/schema";

import type { ActionReadHealth, DockletAdapter, Topic } from "../adapter";
import { AdapterOutcomeError, GENERIC_REASON, HUB_REF, unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import { createDisconnectedAdapter } from "../disconnected-adapter";
import type { CodedActionRun } from "../message-code";
import type { InventoryObservation } from "@/domain/model";
import { createMetricsReader, type MetricsSocket } from "./metrics";
import { createObservedHostImageReader, createResourceReader } from "./resources";
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
import { activityDeadline } from "./activity-deadline";
import { createActivityTargets } from "./activity-targets";
import { mapActivityDetail } from "./map-dto";
import { createActivityReader } from "./activity-reader";
import { createEventReader } from "./event-reader";
import { createVolumePrune } from "./volume-prune";
import { createNetworkCommands } from "./network-commands";
import { createVolumeCommands } from "./volume-commands";
import { createProjectWorkspace } from "./project-workspace";
import { createVolumeWorkspace } from "./volume-workspace";
import { BACKUP_OPERATIONS, createBackups } from "./backups";
import { createVolumeReads } from "./volumes";
import { createStackAuthoring } from "./stack-authoring";

type ReadOperation = "get-capabilities" | "listEnvironments" | "list-activities" | "get-activity" | "listEvents";
type Body<I extends ReadOperation> = operations[I]["responses"][200]["content"]["application/json"];
type StreamPayload = operations["streamClient"]["responses"][200]["content"]["text/event-stream"][number]["data"];
type Source = Pick<EventSource, "close" | "onopen" | "onmessage" | "onerror"> & Partial<Pick<EventSource, "readyState">>;

const IMPLEMENTED = new Set<string>([...BACKUP_OPERATIONS,"getSecurity", "runScan", "setScanEnabled", "getScanSettings", "saveScanSettings", "cancelScan", "deleteRestorePoint", "deleteHostRestorePoint", "listHostRestorePoints", "getUpdateBaseline", "listRestorePoints", "getAutoUpdatePolicy", "saveAutoUpdatePolicy", "setAutoUpdate", "setAutoUpdateApply", "getUpdateSchedules", "saveUpdateSchedule", "listSchedule", "getSystemImageHints", "listSystemComponents", "checkStackUpdates", "checkUpdates", "installStackUpdates", "restoreRestorePoint", "clearContainerRegistryCredential", "pullImage", "removeImage", "pruneImages", "listContainerRegistries", "createContainerRegistry", "updateContainerRegistry", "deleteContainerRegistry", "testContainerRegistry", "createNetwork", "removeNetwork", "connectNetwork", "disconnectNetwork", "getProjectWorkspace", "readProjectFile", "downloadProjectFile", "updateProjectWorkspace", "listFiles", "readFile", "writeFile", "createEntry", "renameEntry", "deleteEntry", "uploadFile", "downloadFile", "planVolumePrune", "pruneVolumes", "createVolume", "deleteVolume", "renameVolume", "subscribeLogs", "subscribeContainerLogs", "runStackCommand", "runContainerCommand", "deployStack", "saveStackFiles", "setStackLabels", "convertDockerRun", "registerHost", "issueAgentToken", "removeHost", "discoverHub", "listStacks", "getStack", "listContainers", "listHosts", "listActions", "listEvents", "subscribeActions", "subscribeChanges", "getAuthMode", "getCurrentUser", "login", "logout", "changePassword", "updateMyLocale", "getSetupStatus", "completeSetup", "getConnectedSettings", "saveSettings", "listImages", "listVolumes", "getVolume", "getTopology", "subscribeMetrics"]);
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
    const path = new URL(input instanceof Request ? input.url : String(input), (typeof window === "undefined" ? "https://hub.example.com" : window.location.href)).pathname;
    // Auth and activity/runtime consumers already own their authoritative 401 path.
    const ownedRejection = ["/auth/login", "/auth/password", "/auth/me"].some((suffix) => path.endsWith(suffix)) || /\/(activities|runtime-runs)(\/|$)|\/(capabilities|environments)$/.test(path);
    if (version === identityGeneration && response.status === 401 && !ownedRejection) {
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
  let stopIdentityCheck: (() => void) | undefined;
  let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

  async function readBody<I extends ReadOperation>(operationId: I, route: keyof paths | string, query?: Record<string, string | number>, observeResponse?: (response: Response) => void, signal?: AbortSignal): Promise<Body<I>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query ?? {})) params.set(key, String(value));
    const url = `${baseUrl}${route}${params.size ? `?${params}` : ""}`;
    let response: Response;
    try {
      response = await fetcher(url, { credentials: "same-origin", headers: { Accept: "application/json" }, signal });
    } catch (error) { throw classifiedError(error); }
    if (response.status === 401) throw classifiedError(null, 401);
    let body: unknown;
    try { body = await response.json(); } catch (error) { throw classifiedError(error, response.status); }
    if (!response.ok) throw classifiedError(body, response.status);
    if (!body || typeof body !== "object" || (body as { success?: unknown }).success !== true || !("data" in body)) {
      throw classifiedError({ code: `${operationId}.invalid-response` }, response.status);
    }
    observeResponse?.(response);
    return body as Body<I>;
  }

  function read<I extends ReadOperation>(operationId: I, route: keyof paths | string, query?: Record<string, string | number>, observeResponse?: (response: Response) => void, signal?: AbortSignal): Promise<Body<I>> {
    return operationId === "listEvents"
      ? readBody(operationId, route, query, observeResponse, signal)
      : activityDeadline((owned) => readBody(operationId, route, query, observeResponse, owned), signal);
  }

  const capabilityReads = new Map<string, Promise<Body<"get-capabilities">["data"]>>();
  let environmentState: string | undefined;
  const inventoryTopics: Topic[] = ["stacks", "containers", "images", "volumes", "topology", "security"];
  const environmentCache = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 10_000, gcTime: 30_000 } } });
  const environmentsKey = ["environments"];
  const environments = () => environmentCache.fetchQuery({ queryKey: environmentsKey, queryFn: () => page("listEnvironments", "/environments", 100) });
  function invalidateCapabilities() { capabilityReads.clear(); }
  function invalidateEnvironments() { sharedReads.clear(); void environmentCache.invalidateQueries({ queryKey: environmentsKey }); }
  function suspendIdentity(retain = false, nextActive = false) {
    ++identityGeneration;
    backups.invalidate();
    events.identityChanged();
    authenticated = nextActive;
    stopIdentityCheck?.(); stopIdentityCheck = undefined;
    identityCheck = undefined;
    runtime.reset(retain);
    targets.reset(retain);
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
    return JSON.stringify(rows.map((row) => [row.id, row.connectedAt, row.edgeSessionId, row.edgeAgentInstance, row.status, row.enabled, row.apiUrl, row.isEdge, row.agentReport?.version.currentVersion, row.agentReport?.version.agentContractVersion, row.agentReport?.version.agentContractFingerprint]).sort((a, b) => String(a[0]).localeCompare(String(b[0]))));
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
      // Transport loss or a server failure can recover; a refusal or a missing or denied descriptor cannot.
      const status = error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? error.outcome.status : 0;
      const lost = error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.code === "http.error" && (status === undefined || status < 400 || status >= 500);
      return fallback(lost ? "capability.read-unavailable" : GENERIC_REASON);
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

  async function page<I extends "listEnvironments" | "list-activities" | "listEvents">(operationId: I, route: string, limit: number, observeResponse?: (response: Response) => void, signal?: AbortSignal): Promise<NonNullable<Body<I>["data"]>> {
    const result: NonNullable<Body<I>["data"]> = [] as unknown as NonNullable<Body<I>["data"]>;
    let start = 0;
    for (;;) {
      if (signal?.aborted) throw new CancelledError();
      let responseIncomplete = false;
      const body = await read(operationId, route, { start, limit, order: "desc", ...(operationId === "list-activities" ? { includeSummary: "true" } : {}) }, (response) => { responseIncomplete = response.headers.get("X-Docklet-Host-Reason") === "agent.unavailable"; observeResponse?.(response); }, signal);
      if (!Array.isArray(body.data) || !body.pagination || !Number.isInteger(body.pagination.totalItems)) throw classifiedError({ code: `${operationId}.invalid-page` });
      result.push(...body.data as never[]);
      start += body.data.length;
      if (limit === -1 && start < body.pagination.totalItems && responseIncomplete === false) throw classifiedError({ code: `${operationId}.short-page` });
      if (limit === -1 || start >= body.pagination.totalItems) return result;
      if (body.data.length === 0) throw classifiedError({ code: `${operationId}.short-page` });
    }
  }

  async function checkIdentity(): Promise<boolean> {
    if (identityCheck) return identityCheck;
    const version = identityGeneration;
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 10_000);
    const stop = () => { clearTimeout(timeout); controller.abort(); };
    stopIdentityCheck = stop;
    const pending = (async () => {
      try {
        const response = await client.fetch(`${baseUrl}/auth/me`, { signal: controller.signal, credentials: "same-origin" });
        await response.arrayBuffer();
        if (version !== identityGeneration) return false;
        if (response.status === 401) { suspendIdentity(); return false; }
        if (!response.ok) throw classifiedError(null, response.status);
        return authenticated;
      } finally { clearTimeout(timeout); if (stopIdentityCheck === stop) stopIdentityCheck = undefined; }
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
  const scanRequest = vulnerabilityTransport(baseUrl, client, () => identityGeneration);
  const runtime = createRuntimeCommands({ baseUrl, client, capability: getCapability, unauthorized: () => { suspendIdentity(); actions.reset(true); }, publish: (run, detailed) => { actions.upsert(run); if (detailed) targets.accept(run); } });
  const actions = createActivityReader({
    hasConsumer: () => actionListeners.size > 0,
    retained: runtime.retained,
    mergeSummary: runtime.mergeSummary,
    changed: (previous, current, recovered) => {
      targets.changed(previous, current, recovered);
      if (current.some((run) => run.kind === "scan" && !["running", "queued"].includes(run.status) && previous.some((old) => old.id === run.id && old.hostId === run.hostId && ["running", "queued"].includes(old.status)))) securityChanged();
    },
    detailRun: async (host, id, signal, expanded) => {
      if (expanded && actions.runs().some((run) => run.id === id && run.hostId === host && run.outcomeState !== undefined)) return runtime.detail(host, id, false, false, signal);
      const backupOwner = backups.guard(host);
      const dto = (await read("get-activity", `/environments/${encodeURIComponent(host)}/activities/${encodeURIComponent(id)}`, { limit: 500 }, undefined, signal)).data!;
      try {
        const run = mapActivityDetail(dto);
        if (run.kind === "backup" || run.kind === "volume-restore") {
          backupOwner();
          try {
            const overview = await backups.api.getBackupOverview(host);
            backupOwner();
            const row = overview.volumes.find((v) => v.lastBackup?.actionId === id || v.lastRestore?.activityId === id);
            const outcome = row?.lastRestore?.activityId === id ? { status: row.lastRestore.status, errorCode: row.lastRestore.code } : row?.lastBackup ? { status: row.lastBackup.status, errorCode: row.lastBackup.errorCode } : undefined;
            if (outcome) run.backupOutcome = outcome;
          } catch (error) { if (isCancelledError(error)) throw error; }
          if (!["running", "queued"].includes(run.status)) backupChanged();
        }
        if (run.kind === "scan" && dto.activity.resourceType === "environment") {
          try {
            const capability = await getCapability("getSecurity", { kind: "host", id: host });
            if (!capability.available) throw unavailable("getSecurity", capability.resourceRef, capability.disabledReasonCode);
            run.scanProgress = await readScanProgress(scanRequest, host, id, signal);
          } catch (error) { if (isCancelledError(error)) throw error; }
        }
        // Invalidate before publishing terminal status: progress consumers may unmount immediately.
        if (run.kind === "scan" && !["running", "queued"].includes(run.status)) securityChanged();
        return run;
      } catch (error) { if (isCancelledError(error)) throw error; throw classifiedError({ code: "activity.invalid-data" }); }
    },
    hosts: async () => {
      await requireCapability("listActions");
      return [...new Set(["0", ...(await environments()).map((item) => item.id)])];
    },
    page: async (hostId, signal) => {
      let incomplete = false;
      const items = await page("list-activities", `/environments/${encodeURIComponent(hostId)}/activities`, -1, (response) => {
        if (response.headers.get("X-Docklet-Host-Reason") === "agent.unavailable") incomplete = true;
      }, signal);
      return { items, incomplete };
    },
    unauthorized: suspendIdentity,
    detail: async (hostId, id, signal) => (await read("get-activity", `/environments/${encodeURIComponent(hostId)}/activities/${encodeURIComponent(id)}`, { limit: 500 }, undefined, signal)).data!,
    publish: (runs, health) => {
      healthListeners.forEach((listener) => listener(health));
      if (runs) actionListeners.forEach((listener) => listener(runs));
    },
  });

  const targets = createActivityTargets({ terminal: (host) => refreshHost(host), hasConsumer: () => actionListeners.size > 0, read: (host, id, signal, expanded) => actions.readDetail(id, host, signal, expanded) });

  const events = createEventReader(async () => {
    await requireCapability("listEvents");
    const rows = await page("listEvents", "/events", 100);
    try { return rows.map(mapEvent); } catch (error) { throw classifiedError({ code: "listEvents.mapping-error", cause: error }); }
  }, () => identityGeneration, () => hasReadConsumer(adapter.subscribeChanges, "events"));
  const loadEvents = events.load;

  function refreshRuns(recovery = false): void {
    if (actionListeners.size === 0) return;
    void actions.refresh(recovery).catch((error) => { if (isCancelledError(error)) return; window.dispatchEvent(new CustomEvent("docklet:adapter-error", { detail: error })); });
  }

  function refreshHost(host: string): void {
    if (actionListeners.size === 0) return;
    if (!actions.hasHost(host)) invalidateEnvironments();
    void actions.refreshHost(host).catch(() => undefined);
  }

  function refreshRun(activityId: string, environmentId?: string, terminal?: string, discoverOnTerminal = false): void {
    if (actionListeners.size === 0) return;
    const previous = actions.runs().find((run) => run.id === activityId && run.hostId === environmentId);
    if (!previous || !environmentId) { if (environmentId) refreshHost(environmentId); else refreshRuns(); return; }
    targets.notify(environmentId, activityId, terminal, discoverOnTerminal);
  }

  function securityChanged() { sharedReads.clear(); changeListeners.forEach((listener) => listener(["security", "images", "settings"])); }

  function refreshSubscribed(reopen = false): void {
    invalidateEnvironments();
    events.changed();
    refreshRuns(reopen);
    if (changeListeners.size) changeListeners.forEach((listener) => listener(["events", "hosts", "stacks", "containers", "images", "volumes", "topology", "security"]));
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
      const notification = envelope.activity;
      if (notification.type !== "message" && notification.activity?.type === "vulnerability_scan") securityChanged();
      if (notification.type !== "message" && notification.activity?.metadata?.action && /backup|restore/.test(String(notification.activity.metadata.action))) backupChanged();
      const host = notification.environmentId ?? notification.activity?.environmentId;
      if (notification.type === "snapshot" || notification.type === "error") {
        if (host) {
          if (notification.type === "error") actions.hostError(host);
          refreshHost(host);
        } else refreshRuns();
      } else {
        const activityId = notification.activityId ?? notification.activity?.id;
        const status = notification.activity?.status;
        const terminal = status && status !== "queued" && status !== "running" ? `${status}:${notification.activity?.endedAt ?? ""}` : undefined;
        if (activityId) refreshRun(activityId, host, terminal, notification.type !== "message");
        else if (host) refreshHost(host);
        else refreshRuns();
      }
      return;
    }
    if (envelope.channel === "events" && envelope.eventLog && typeof envelope.eventLog.type === "string") {
      events.changed();
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
      if (changed) { backups.invalidate(); refreshRuns(); }
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
      refreshSubscribed(true);
    };
    opened.onmessage = (event: MessageEvent) => { if (source === opened && version === identityGeneration) handleStream(event.data, event.lastEventId); };
    opened.onerror = () => {
      if (source !== opened || version !== identityGeneration) return;
      lastEventId = undefined;
      actions.markError();
      // A reconnecting EventSource keeps its native retry cadence. Only CLOSED
      // sources need a replacement after the authoritative identity check.
      const closed = opened.readyState === 2;
      if (closed) { opened.close(); source = undefined; sourceOpen = false; }
      void checkIdentity().then((valid) => { if (closed && valid && version === identityGeneration) scheduleReconnect(); }, () => { if (closed && version === identityGeneration) scheduleReconnect(); });
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
    stopIdentityCheck?.(); stopIdentityCheck = undefined; identityCheck = undefined;
    source?.close();
    source = undefined;
    sourceOpen = false;
    actions.invalidate();
    targets.reset();
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
  function backupChanged() { sharedReads.clear(); changeListeners.forEach((listener) => listener(["volumes", "settings", "schedule"])); }
  const backups = createBackups({ baseUrl, client, capability: getCapability, generation: () => identityGeneration, hosts: listHosts, changed: backupChanged, accepted: (host, id) => { refreshHost(host); targets.notify(host, id, undefined, true); } });
  const volumeReads = createVolumeReads(baseUrl, fetcher, listHosts, requireCapability, observeHost, () => identityGeneration);
  const authoring = createStackAuthoring({
    baseUrl, client, capability: getCapability,
    up: (stackId) => runtime.runStackCommand(stackId, "start"),
    labels: async (stackId) => (await inventory.getStack(stackId)).labels,
    changed: () => changeListeners.forEach((listener) => listener(["stacks"])),
    unauthorized: () => { suspendIdentity(); actions.reset(true); },
  });
  const unsupported = createDisconnectedAdapter();
  const methods = Object.fromEntries(Object.keys(unsupported).filter((key) => key !== "kind").map((operationId) => [operationId, (...args: unknown[]) => Promise.reject(unavailable(operationId, refFor(operationId, args), "capability.mapping-unavailable"))])) as unknown as DockletAdapter;
  const adapter: DockletAdapter = {
    ...methods,
    ...createHttpAuth(baseUrl, client, invalidateIdentity, () => identityGeneration),
    ...createHostCommands(baseUrl, client, requireCapability, () => { backups.invalidate(); invalidateCapabilities(); invalidateEnvironments(); changeListeners.forEach((listener) => listener(["hosts", ...inventoryTopics])); }),
    ...createStackUpdates({ baseUrl, client, capability: getCapability, generation: () => identityGeneration, changed: () => { sharedReads.clear(); changeListeners.forEach((listener) => listener(["stacks", "containers", "images", "schedule"])); } }),
    installStackUpdates: runtime.installStackUpdates,
    restoreRestorePoint: runtime.restoreRestorePoint,
    runStackCommand: runtime.runStackCommand,
    runContainerCommand: runtime.runContainerCommand,
    getRuntimeRun: runtime.getRuntimeRun,
    cancelRuntimeRun: runtime.cancelRuntimeRun,
    releaseRuntimeRun: runtime.releaseRuntimeRun,
    ...authoring,
    ...createSettingsCore(baseUrl, client),
    ...createHttpHubBackups(baseUrl, client, options.uploadFactory),
    kind: "http",
    mappedOperations: IMPLEMENTED,
    getCapability,
    listHosts,
    ...inventory,
    ...resourceReads,
    ...createVulnerabilities({ baseUrl, client, generation: () => identityGeneration, capability: getCapability, hosts: listHosts, images: createObservedHostImageReader(baseUrl, fetcher, () => {}), changed: securityChanged, accepted: (host, id) => { refreshHost(host); targets.notify(host, id, undefined, true); } }),
    ...volumeReads,
    ...backups.api,
    async listVolumes(onPartial) {
      const rows = await volumeReads.listVolumes(onPartial);
      const hosts = [...new Set(rows.filter((v) => v.kind === "volume").map((v) => v.hostId))];
      const mapped = await Promise.all(hosts.map((host) => backups.decorate(host, rows.filter((v) => v.hostId === host))));
      const covered = new Set(hosts);
      return [...mapped.flat(), ...rows.filter((v) => !covered.has(v.hostId))];
    },
    async getVolume(id) {
      let volume;
      try { volume = await volumeReads.getVolume(id); }
      catch (error) {
        const missing = error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status === 404 || error instanceof Error && error.message === "not-found";
        if (!missing) throw error;
        const host = id.split("~")[0]!;
        const observed = (await backups.api.getBackupOverview(host)).volumes.find((v) => v.id === id && v.volumeExists === false);
        if (!observed) throw error;
        volume = { ...observed, backups: [] };
      }
      if (volume.kind === "bind") return volume;
      const [decorated, history] = await Promise.all([backups.decorate(volume.hostId, [volume]), backups.history(id).catch(() => null)]);
      return { ...volume, ...decorated[0], backups: history ?? [], backupUnavailable: !history || decorated[0]?.backupUnavailable };
    },
    ...createImageCommands({ baseUrl, client, capability: getCapability, generation: () => lifetimeGeneration, changed: () => { sharedReads.clear(); changeListeners.forEach((listener) => listener(["images"])); } }),
    ...createRegistryCommands({ baseUrl, client, capability: getCapability, generation: () => lifetimeGeneration, changed: () => { sharedReads.clear(); changeListeners.forEach((listener) => listener(["registries"])); } }),
    ...createProjectWorkspace({ baseUrl, client, capability: getCapability, generation: () => identityGeneration }),
    ...createVolumeWorkspace({ baseUrl, client, capability: getCapability, generation: () => identityGeneration }),
    ...createVolumePrune({ baseUrl, client, capability: getCapability, changed: () => { sharedReads.clear(); changeListeners.forEach((listener) => listener(["volumes"])); } }, listHosts),
    ...createNetworkCommands({ baseUrl, client, capability: getCapability, changed: () => { sharedReads.clear(); changeListeners.forEach((listener) => listener(["topology", "containers"])); } }),
    ...createVolumeCommands({ baseUrl, client, capability: getCapability, changed: () => { sharedReads.clear(); changeListeners.forEach((listener) => listener(["volumes"])); } }),
    async listSchedule(hostIds) {
      const selected = hostIds ?? (await listHosts()).map((h) => h.id);
      const backupRows = (await Promise.all(selected.map(async (host) => {
        try { return (await backups.api.getBackupOverview(host)).volumes.filter((v) => v.backup?.enabled && v.backup.nextRunAt).map((v) => ({ id: `${host}~backup-${v.name}`, hostId: host, kind: "backup" as const, title: v.name, volumeId: v.id, cadence: v.backup!.schedule.mode, nextRunAt: v.backup!.nextRunAt, enabled: true })); }
        catch { return []; }
      }))).flat();
      return [...backupRows, ...(await Promise.all(selected.map(async (hostId) => {
        try {
          const config = await adapter.getUpdateSchedules!(hostId);
          return config.jobs.filter((j) => j.kind === "auto-update" || j.kind === "update-check").map((j) => ({ id: `${hostId}~${j.id}`, hostId, kind: j.kind as "auto-update" | "update-check", title: "", cadence: j.schedule, nextRunAt: j.nextRun, enabled: j.enabled, code: j.code }));
        } catch (error) {
          if (isCancelledError(error)) throw error;
          return [{ id: `${hostId}~schedule-unavailable`, hostId, kind: "update-check" as const, title: "", cadence: "", unavailable: true }];
        }
      }))).flat()];
    },
    listActions: actions.load,
    observeActionDetail(host, id, listener) {
      const offActions = adapter.subscribeActions(() => {});
      const owner = lifetimeGeneration;
      let active = true;
      const pollUntil = Date.now() + 30 * 60_000;
      let scanTimer: ReturnType<typeof setInterval> | undefined;
      const stopPolling = () => { clearInterval(scanTimer); scanTimer = undefined; };
      const pollScan = (run?: CodedActionRun) => {
        if (!run || !["scan", "backup", "volume-restore"].includes(run.kind) || !["running", "queued"].includes(run.status)) { stopPolling(); return; }
        if (scanTimer || Date.now() >= pollUntil) return;
        // Scan counters change without activity events. Share the existing owned detail read.
        scanTimer = setInterval(() => {
          const current = actions.runs().find((row) => row.hostId === host && row.id === id);
          if (!active || owner !== lifetimeGeneration || Date.now() >= pollUntil || !current || !["running", "queued"].includes(current.status)) { stopPolling(); return; }
          targets.refreshObserved(host, id);
        }, 3_000);
      };
      runtime.observeDetail(id, true);
      pollScan(actions.runs().find((run) => run.hostId === host && run.id === id));
      const off = targets.observe(host, id, (state) => {
        if (state.status === "ready") pollScan(state.run);
        listener(state);
      });
      return () => { if (!active) return; active = false; stopPolling(); off(); if (owner === lifetimeGeneration) runtime.observeDetail(id, false); offActions(); };
    },
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
      return () => { if (!actionListeners.delete(listener)) return; if (!actionListeners.size) { targets.dropIdle(); runtime.observe(false); } disconnectIfUnused(); };
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
  registerIdentityLifetime(adapter.subscribeChanges, {
    version: () => lifetimeGeneration,
    active: () => authenticated,
    listen: (notify) => { identityListeners.add(notify); return () => { identityListeners.delete(notify); }; },
  });
  // Guard public reads and partial publishers in addition to transport bodies.
  return new Proxy(adapter, { get(target, key, receiver) {
    const method: unknown = Reflect.get(target, key, receiver);
    if (typeof method !== "function" || !/^(list|get|discover|planVolumePrune)/.test(String(key))) return method;
    return (...args: unknown[]) => {
      // Auth and activities already implement their specialized renewal re-read.
      if (key === "getCurrentUser" || key === "getAuthMode" || key === "listActions") return method(...args);
      const owner = lifetimeGeneration;
      const invoke = async (retryRenewal = true): Promise<unknown> => {
        const version = identityGeneration;
        const check = () => { if (owner !== lifetimeGeneration || version !== identityGeneration) throw new CancelledError(); };
        const wrapped = args.map((arg) => typeof arg === "function" ? (...values: unknown[]) => { check(); return arg(...values); } : arg);
        try {
          const value: unknown = await method(...wrapped);
          check();
          return value;
        } catch (error) {
          if (owner !== lifetimeGeneration) throw new CancelledError();
          if (retryRenewal && authenticated && version !== identityGeneration) return invoke(false);
          throw error;
        }
      };
      return invoke();
    };
  } });
}
