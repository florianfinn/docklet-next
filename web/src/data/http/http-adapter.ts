// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original HTTP and SSE adapter for issue 38; notification-scoped activity/event refreshes and effective sequential event pages (#211). M14 host maintenance and availability. Simple-user management, assigned-stack access and mode-aware self-service. M18 #341 #343 #342 #344 #250 #346 #319: v14 policies, explicit update/recovery gestures and shared scheduling rule. M19 #348 #349 #353 #356: live import, bulk actions, Auto Heal cooldown and global search. M20 #360: shell list reads and target-specific terminal invalidation. D4 #271: fair physical host reads and result-local recovery. M21 #439: Compose self-update controls and capability integration. M21 #437: advisory detail. M21 #438: candidate checks and verified updates. M23 #389 #391: dispatch gate (maintenance hold, unavailable-host skips).

import { createSelfUpdates } from "./self-update";
import { inventoryReadBudget } from "./host-read-deadline";
import { inventoryId } from "./inventory-mapping";
import { createImageMetadata } from "./image-metadata";
import { createStackImport } from "./stack-import";
import { nextOccurrence } from "@/domain/schedule";
import { vulnerabilityTransport } from "./vulnerability-transport";
import { createVulnerabilities, readScanProgress } from "./vulnerabilities";
import { createUpdateSettings } from "./update-settings";
import { createStackUpdates } from "./stack-updates";
import { createImageCommands } from "./image-commands";
import { createRegistryCommands } from "./registry-commands";

import { hasReadConsumer } from "../read-consumers";
import { CancelledError, isCancelledError, QueryClient } from "@tanstack/react-query";

import type { components, operations, paths } from "@/api/generated/schema";

import type { ActionReadHealth, DockletAdapter, Topic } from "../adapter";
import { AdapterOutcomeError, GENERIC_REASON, HUB_REF, unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import { createDisconnectedAdapter } from "../disconnected-adapter";
import type { CodedActionRun } from "../message-code";
import type { InventoryObservation } from "@/domain/model";
import { backupAccess } from "@/domain/volumes";
import { createMetricsReader, type MetricsSocket } from "./metrics";
import { createObservedHostImageReader, createResourceReader } from "./resources";
import type { CurrentUser } from "../api";
import { createHttpUsers, USER_OPERATIONS } from "./users";
import { createUserInventory } from "./user-inventory";
import { createInventoryReader } from "./inventory";
import { createConditionReader } from "./runtime-conditions";
import { mapHost } from "./hosts";
import { createSystemComponents } from "./system-components";
import { createHostRestarts } from "./host-restarts";
import { createHostChecks } from "./host-checks";
import { createHostCommands } from "./host-commands";
import { mapEvent, mapStatus } from "./map-dto";
import { createHttpHubBackups } from "./hub-backups";
import { createHttpHistory, createHttpUiPreferences } from "./history";
import { createHttpServiceWidgets } from "./service-widgets";
import { problemCode } from "./problem-code";
import { createTransportChain } from "./transport-chain";
import { createHttpAuth, type SessionChange } from "./auth";
import { createSettingsCore } from "./settings-core";
import { createSharedReads } from "./shared-reads";
import { createRuntimeCommands } from "./runtime-commands";
import { createShellSockets, type ShellSocket } from "./shell";
import { createRuntimeLogs } from "./logs";
import { createVulnerabilityCandidates } from "./vulnerability-candidates";
import { createVulnerabilityAdvisories } from "./vulnerability-advisory";
import { registerIdentityLifetime } from "../identity-lifetime";
import { activityDeadline } from "./activity-deadline";
import { createActivityTargets } from "./activity-targets";
import { mapActivityDetail } from "./map-dto";
import { createActivityReader } from "./activity-reader";
import { createEventReader } from "./event-reader";
import { DASHBOARD_EVENT_LIMIT, DASHBOARD_EVENT_WINDOW_MS, createDashboardEvents } from "./dashboard-events";
import { type Body, type ReadOperation, createPager } from "./paged-read";
import { pausableTimeout } from "./deadline-pause";
import { isMaintenanceError } from "../maintenance-retry";
import { createVolumePrune } from "./volume-prune";
import { createNetworkCommands } from "./network-commands";
import { createVolumeCommands } from "./volume-commands";
import { createProjectWorkspace } from "./project-workspace";
import { createVolumeWorkspace } from "./volume-workspace";
import { BACKUP_OPERATIONS, createBackups } from "./backups";
import { createNotificationSettings, NOTIFICATION_SETTINGS_OPERATIONS } from "./notification-settings";
import { createVolumeReads } from "./volumes";
import { createContainerOrigin } from "./container-origin";
import { createStackAuthoring } from "./stack-authoring";
import { createStackTerminals } from "./stack-terminals";

type StreamPayload = operations["streamClient"]["responses"][200]["content"]["text/event-stream"][number]["data"];
type Source = Pick<EventSource, "close" | "onopen" | "onmessage" | "onerror"> & Partial<Pick<EventSource, "readyState">>;

const IMPLEMENTED = new Set<string>([
  ...USER_OPERATIONS,
  ...BACKUP_OPERATIONS,
  ...NOTIFICATION_SETTINGS_OPERATIONS,
  "getSecurity", "getVulnerabilityAdvisory", "ignore-vulnerability", "unignore-vulnerability",
  "runScan", "scan-vulnerability-candidates", "get-vulnerability-candidates",
  "setScanEnabled",
  "getScanSettings",
  "saveScanSettings",
  "cancelScan",
  "deleteRestorePoint", "deleteHostRestorePoint", "listHostRestorePoints",
  "getUpdateBaseline",
  "listRestorePoints",
  "getAutoUpdatePolicy", "saveAutoUpdatePolicy", "setAutoUpdate", "setAutoUpdateApply",
  "listUpdatePolicies",
  "getContainerUpdatePolicy",
  "saveContainerUpdatePolicy",
  "installContainerUpdate",
  "restoreContainerImage",
  "getContainerPreviousImage",
  "checkAllUpdates",
  "getUpdateSettings",
  "listSchedule",
  "getSystemImageHints",
  "listSystemComponents",
  "checkStackUpdates",
  "checkUpdates",
  "installStackUpdates",
  "restoreRestorePoint",
  "clearContainerRegistryCredential",
  "pullImage",
  "removeImage",
  "pruneImages",
  "listContainerRegistries",
  "createContainerRegistry",
  "updateContainerRegistry",
  "deleteContainerRegistry",
  "testContainerRegistry",
  "createNetwork",
  "removeNetwork",
  "connectNetwork",
  "disconnectNetwork",
  "getProjectWorkspace",
  "readProjectFile",
  "downloadProjectFile",
  "updateProjectWorkspace",
  "listFiles",
  "readFile",
  "writeFile",
  "createEntry",
  "renameEntry",
  "deleteEntry",
  "uploadFile",
  "downloadFile",
  "planVolumePrune",
  "pruneVolumes",
  "createVolume",
  "deleteVolume",
  "renameVolume",
  "subscribeLogs",
  "subscribeContainerLogs",
  "openContainerShell",
  "redeployStack",
  "runStackCommand",
  "runContainerCommand",
  "deployStack",
  "saveStackFiles",
  "setStackLabels",
  "convertDockerRun",
  "restartAgent", "selfUpdate", "getSelfUpdate",
  "connectHost", "reconnectHost",
  "updateHost",
  "registerHost",
  "issueAgentToken",
  "removeHost",
  "discoverHub",
  "listStacks",
  "getStack",
  "listContainers",
  "listHosts",
  "listActions",
  "listEvents",
  "subscribeActions",
  "subscribeChanges",
  "getAuthMode",
  "getCurrentUser",
  "login",
  "logout",
  "changePassword",
  "updateMyLocale",
  "getSetupStatus",
  "completeSetup",
  "getConnectedSettings",
  "saveSettings",
  "setContainerOrigin",
  "recreateContainer",
  "restartService",
  "importSource", "getImageMetadata",
  "listImages",
  "listVolumes",
  "getVolume",
  "getTopology",
  "subscribeMetrics",
]);
const RECONNECT_MS = 250;
// Longest wait of a logout for answers to reads sent before it.
const LOGOUT_DRAIN_MS = 5_000;
// Public reads of `/auth/...` routes keep their own outcomes after a sign-out.
const AUTH_READS = new Set(["getAuthConfiguration", "getUiPreferences"]);

export interface HttpAdapterOptions {
  baseUrl?: string;
  fetch?: typeof globalThis.fetch;
  eventSource?: (url: string) => Source;
  webSocket?: (url: string) => MetricsSocket;
  shellWebSocket?: (url: string) => ShellSocket;
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
  return new AdapterOutcomeError({ kind: "error", code: problemCode(error) ?? "http.error", status });
}

/** A small typed caller; response shape is checked before a DTO reaches the mapper. */
export function createHttpAdapter(options: HttpAdapterOptions = {}): DockletAdapter {
  const baseUrl = (options.baseUrl ?? "/api").replace(/\/$/, "");
  // The outcome of a running logout (true: signed out), and whether reads stay closed until the next sign-in.
  let closing: Promise<boolean> | undefined;
  let signedOut = false;
  /** The one admission rule for every physical non-auth read and socket handshake: wait while a logout runs, refuse
   * once it signed out. Requests dispatched earlier keep running. */
  async function admit(): Promise<void> {
    if (closing && await closing) throw new CancelledError();
    if (signedOut) throw new CancelledError();
  }
  const { maintenance, hostPolicy, identityReads, answering, client } = createTransportChain({
    baseUrl, fetch: options.fetch ?? globalThis.fetch, admit, closing: () => !!closing || signedOut,
    generation: () => identityGeneration, suspend: () => suspendIdentity(),
  });
  // A hub backup download is a browser navigation outside fetch: it waits for logout admission and maintenance itself.
  const hubBackups = createHttpHubBackups(baseUrl, client, options.uploadFactory, async () => {
    await admit();
    const version = identityGeneration;
    await maintenance.released();
    if (version !== identityGeneration) throw new CancelledError();
    await admit();
  });
  const sharedReads = createSharedReads(client.fetch);
  const fetcher = sharedReads.fetch;
  const sourceFactory = options.eventSource ?? ((url: string) => new EventSource(url, { withCredentials: true }));
  const changeListeners = new Set<(topics: Topic[]) => void>();
  const stackListeners = new Set<(target: { stackId: string; hostId: string }) => void>();
  const actionListeners = new Set<(runs: CodedActionRun[]) => void>();
  let source: Source | undefined;
  const healthListeners = new Set<(health: ActionReadHealth) => void>();
  let lastEventId: number | undefined;
  let sourceOpen = false;
  let authenticated = true;
  let principal: CurrentUser | undefined;
  let identityGeneration = 0;
  let lifetimeGeneration = 0;
  // Aborted when another identity takes over, so a pending or retried owner write never runs as the next user.
  let ownedWrites = new AbortController();
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
  const environments = () => environmentCache.fetchQuery({ queryKey: environmentsKey, queryFn: async () => {
    const token = hostPolicy.beginCatalogue();
    const rows = await page("listEnvironments", "/environments", 100);
    hostPolicy.applyCatalogue(token, rows);
    return rows;
  } });
  function invalidateCapabilities() { capabilityReads.clear(); }
  function invalidateEnvironments() { sharedReads.clear(); void environmentCache.invalidateQueries({ queryKey: environmentsKey }); }
  function suspendIdentity(retain = false, nextActive = false) {
    ++identityGeneration;
    backups.invalidate();
    events.identityChanged(); dashboardEvents.identityChanged();
    authenticated = nextActive;
    stopIdentityCheck?.(); stopIdentityCheck = undefined;
    identityCheck = undefined;
    runtime.reset(retain);
    stackTerminals.reset();
    restarts.reset();
    if (!retain) selfUpdates.reset();
    targets.reset(retain);
    logs.reset();
    shells.reset();
    metrics.reset(false, retain);
    identityReads.reset();
    maintenanceSeen = false; maintenance.reset(); hostPolicy.reset();
    conditions.reset();
    sharedReads.clear(); invalidateCapabilities(); environmentCache.clear(); observations.clear();
    source?.close(); source = undefined; sourceOpen = false;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    reconnectTimer = undefined;
    if (!retain) { actions.reset(true); ++lifetimeGeneration; ownedWrites.abort(); ownedWrites = new AbortController(); identityListeners.forEach((notify) => notify()); }
  }
  function invalidateIdentity(change: SessionChange) {
    if (change === "established" || change === "renewed") signedOut = false;
    if (change === "confirmed" && authenticated) return;
    const retain = change === "renewed" && authenticated;
    suspendIdentity(retain, change !== "lost");
    if (retain) actions.invalidate(true); else actions.reset(change === "lost");
    authenticated = change !== "lost";
    if (authenticated) { metrics.reset(true, retain); connect(); }
  }
  function environmentKey(rows: components["schemas"]["EnvironmentEnvironment"][]) {
    return JSON.stringify(rows.map((row) => [
      row.id,
      row.connectedAt,
      row.edgeSessionId,
      row.edgeAgentInstance,
      row.status,
      row.enabled,
      row.apiUrl,
      row.isEdge,
      // Credential rotation and Edge re-pairing keep every other field; the revision still discards held reads.
      row.registrationRevision,
      row.agentReport?.version.currentVersion,
      row.agentReport?.version.agentContractVersion,
      row.agentReport?.version.agentContractFingerprint,
    ]).sort((a, b) => String(a[0]).localeCompare(String(b[0]))));
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
      // Hub maintenance decides nothing about the capability: the caller meets the maintenance outcome.
      if (isMaintenanceError(error)) throw error;
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

  const page = createPager(read, classifiedError);

  async function checkIdentity(): Promise<boolean> {
    if (identityCheck) return identityCheck;
    const version = identityGeneration;
    const controller = new AbortController();
    const stopTimer = pausableTimeout(10_000, () => controller.abort());
    const stop = () => { stopTimer(); controller.abort(); };
    stopIdentityCheck = stop;
    const pending = (async () => {
      try {
        const response = await client.fetch(`${baseUrl}/auth/me`, { signal: controller.signal, credentials: "same-origin" });
        await response.arrayBuffer();
        if (version !== identityGeneration) return false;
        if (response.status === 401) { suspendIdentity(); return false; }
        if (!response.ok) throw classifiedError(null, response.status);
        return authenticated;
      } finally { stopTimer(); if (stopIdentityCheck === stop) stopIdentityCheck = undefined; }
    })();
    identityCheck = pending;
    void pending.finally(() => { if (identityCheck === pending) identityCheck = undefined; }).catch(() => undefined);
    return pending;
  }
  const metrics = createMetricsReader(baseUrl, fetcher, listHosts, requireCapability, options.webSocket ?? ((url) => new WebSocket(url)), observeHost, checkIdentity, admit);
  const logs = createRuntimeLogs({
    baseUrl, socket: options.webSocket ?? ((url) => new WebSocket(url)), capability: getCapability,
    authenticated: () => authenticated, admit,
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
  const shells = createShellSockets({ baseUrl, socket: options.shellWebSocket ?? ((url) => new WebSocket(url)), fetch: fetcher, authenticated: () => authenticated, admit, capability: (id) => getCapability("openContainerShell", { kind: "container", id }) });
  const scanRequest = vulnerabilityTransport(baseUrl, client, () => identityGeneration);
  const runtime = createRuntimeCommands({ baseUrl, client, capability: getCapability, unauthorized: () => { suspendIdentity(); actions.reset(true); }, publish: (run, detailed) => { actions.upsert(run); stackTerminals([run], true); if (detailed) targets.accept(run); } });
  const restarts = createHostRestarts({ baseUrl, client, capability: getCapability, generation: () => identityGeneration, publish: (run) => actions.upsert(run), changed: () => { invalidateCapabilities(); invalidateEnvironments(); changeListeners.forEach(listener => listener(["hosts"])); } });
  const stackTerminals = createStackTerminals((target) => { sharedReads.clear(); stackListeners.forEach((listener) => listener(target)); });
  const actions = createActivityReader({
    hasConsumer: () => actionListeners.size > 0,
    retained: () => [...runtime.retained(), ...restarts.retained()],
    mergeSummary: runtime.mergeSummary,
    changed: (previous, current, recovered) => {
      targets.changed(previous, current, recovered);
      if (current.some((run) => run.kind === "scan" && !["running", "queued"].includes(run.status) && previous.some((old) => old.id === run.id && old.hostId === run.hostId && ["running", "queued"].includes(old.status)))) securityChanged();
    },
    detailRun: async (host, id, signal, expanded) => {
      if (restarts.has(host, id)) return restarts.detail(host, id, signal);
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
            const outcome = row?.lastRestore?.activityId === id
              ? {
                  status: row.lastRestore.status,
                  errorCode: row.lastRestore.code,
                  reason: row.lastRestore.reason,
                }
              : row?.lastBackup
                ? { status: row.lastBackup.status, errorCode: row.lastBackup.errorCode }
                : undefined;
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
        if ((run.kind === "scan" || run.sourceOperation === "vulnerability.candidate-scan") && !["running", "queued"].includes(run.status)) securityChanged();
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
      if (runs) { stackTerminals(runs); actionListeners.forEach((listener) => listener(runs)); }
    },
  });

  const selfUpdates = createSelfUpdates({ baseUrl, client, capability: getCapability, generation: () => lifetimeGeneration, readRun: async (host, id, signal) => { const run = await actions.readDetail(id, host, signal, true); actions.upsert(run); return run; } });
  const targets = createActivityTargets({ terminal: (host, run) => { stackTerminals([run], true); refreshHost(host); }, hasConsumer: () => actionListeners.size > 0 || stackListeners.size > 0, read: (host, id, signal, expanded) => principal?.isAdmin === false ? runtime.detail(host, id, false, false, signal) : actions.readDetail(id, host, signal, expanded) });

  const events = createEventReader(async () => {
    await requireCapability("listEvents");
    const rows = await page("listEvents", "/events", 100);
    try { return rows.map(mapEvent); } catch (error) { throw classifiedError({ code: "listEvents.mapping-error", cause: error }); }
  }, () => identityGeneration, () => hasReadConsumer(adapter.subscribeChanges, "events"));
  const loadEvents = events.load;
  const dashboardEvents = createDashboardEvents(async (signal) => {
    await requireCapability("listEvents");
    const body = await read("listEvents", "/events", { since: new Date(Date.now() - DASHBOARD_EVENT_WINDOW_MS).toISOString(), limit: DASHBOARD_EVENT_LIMIT, sort: "timestamp", order: "desc" }, undefined, signal);
    if (!Array.isArray(body.data) || !body.pagination || !Number.isInteger(body.pagination.totalItems)) throw classifiedError({ code: "listEvents.invalid-page" });
    try { return { events: body.data.map(mapEvent), totalItems: body.pagination.totalItems }; } catch (error) { throw classifiedError({ code: "listEvents.mapping-error", cause: error }); }
  }, () => identityGeneration, { hasConsumer: () => hasReadConsumer(adapter.subscribeChanges, "dashboardEvents"), changed: () => changeListeners.forEach((listener) => listener(["dashboardEvents"])) });

  function refreshRuns(recovery = false): void {
    if (principal?.isAdmin === false || actionListeners.size === 0) return;
    void actions.refresh(recovery).catch((error) => { if (isCancelledError(error)) return; window.dispatchEvent(new CustomEvent("docklet:adapter-error", { detail: error })); });
  }

  function refreshHost(host: string): void {
    if (principal?.isAdmin === false || actionListeners.size === 0) return;
    if (!actions.hasHost(host)) invalidateEnvironments();
    void actions.refreshHost(host).catch(() => undefined);
  }

  function refreshRun(activityId: string, environmentId?: string, terminal?: string, discoverOnTerminal = false): void {
    if (principal?.isAdmin === false || !actionListeners.size && !stackListeners.size) return;
    const previous = actions.runs().find((run) => run.id === activityId && run.hostId === environmentId);
    if (!previous || !environmentId) { if (environmentId) refreshHost(environmentId); else refreshRuns(); return; }
    targets.notify(environmentId, activityId, terminal, discoverOnTerminal);
  }

  function securityChanged() { sharedReads.clear(); changeListeners.forEach((listener) => listener(["security", "images", "settings"])); }

  function refreshSubscribed(reopen = false): void {
    invalidateEnvironments();
    events.changed();
    // A new connection's initial event frame, not its open, refreshes the Dashboard's recent events.
    if (!reopen) dashboardEvents.missed();
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
      if (notification.type !== "message" && ["vulnerability_scan", "vulnerability.candidate-scan"].includes(notification.activity?.type ?? "")) securityChanged();
      if (notification.type !== "message"
        && notification.activity?.metadata?.action
        && /backup|restore/.test(String(notification.activity.metadata.action))) backupChanged();
      const host = notification.environmentId ?? notification.activity?.environmentId;
      const activity = notification.activity;
      if (host && activity?.id && activity.resourceId && (activity.resourceType === "project" || activity.resourceType === "stack")
        && activity.status && ["success", "failed", "cancelled"].includes(activity.status)) {
        stackTerminals([{ id: activity.id, hostId: host, stackId: inventoryId(host, activity.resourceId), status: mapStatus(activity.status), finishedAt: activity.endedAt }], true);
      }
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
      dashboardEvents.frame();
      changeListeners.forEach((listener) => listener(["events"]));
      return;
    }
    if (envelope.channel === "environments") {
      const rows = envelope.environment?.environments;
      // The read policy sees the new catalogue before any invalidation dispatches a read.
      if (Array.isArray(rows)) hostPolicy.applyStream(rows);
      invalidateCapabilities();
      invalidateEnvironments();
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
    if (!authenticated || closing || signedOut || source || (!actionListeners.size && !changeListeners.size)) return;
    // The native stream bypasses fetch: it starts only after a hub maintenance hold ends.
    if (maintenance.holding()) { if (!streamHeld) { streamHeld = true; void maintenance.released().then(() => { streamHeld = false; connect(); }); } return; }
    sourceOpen = false;
    let opened: Source;
    try {
      opened = sourceFactory(`${baseUrl}/stream?channels=${principal?.isAdmin === false ? "environments" : "activities%2Cevents%2Cenvironments"}`);
      dashboardEvents.streamStarted(principal?.isAdmin !== false);
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
      const held = maintenance.holding();
      const closed = opened.readyState === 2 || held;
      if (closed) { opened.close(); source = undefined; sourceOpen = false; }
      if (held) { connect(); return; }
      void checkIdentity().then((valid) => { if (closed && valid && version === identityGeneration) scheduleReconnect(); }, () => { if (closed && version === identityGeneration) scheduleReconnect(); });
    };
  }

  let streamHeld = false;
  // After hub maintenance every read may be outdated (a restore replaces the database): refresh all topics once.
  let maintenanceSeen = false;
  maintenance.subscribe((state) => {
    if (state.active) { maintenanceSeen = true; return; }
    if (!maintenanceSeen) return;
    maintenanceSeen = false;
    invalidateCapabilities(); invalidateEnvironments(); backups.invalidate();
    changeListeners.forEach((listener) => listener(["hosts", "stacks", "containers", "security", "topology", "events", "dashboardEvents", "schedule", "volumes", "settings", "images", "registries"]));
  });

  function scheduleReconnect(): void {
    if (!authenticated || closing || signedOut || reconnectTimer || (!actionListeners.size && !changeListeners.size)) return;
    reconnectTimer = setTimeout(() => {
      reconnectTimer = undefined;
      connect();
    }, RECONNECT_MS);
  }

  /** Closes the event stream for a running logout; a failed logout connects again. */
  function pauseStream(): void {
    if (reconnectTimer) clearTimeout(reconnectTimer);
    reconnectTimer = undefined;
    source?.close();
    source = undefined;
    sourceOpen = false;
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
      const hosts = rows.map((row) => ({ ...mapHost(principal?.isAdmin === false ? { ...row, apiUrl: "", enabled: true } : row), inventory: observations.get(row.id) }));
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
  const conditions = createConditionReader(baseUrl, fetcher);
  const inventory = scopedObservations((observe) => createInventoryReader(baseUrl, fetcher, listHosts, requireCapability, observe, conditions.read));
  const userInventory = scopedObservations((observe) => createUserInventory(baseUrl, fetcher, listHosts, () => principal, observe, conditions.read));
  const resourceReads = scopedObservations((observe) => createResourceReader(baseUrl, fetcher, listHosts, requireCapability, observe));
  function backupChanged() { sharedReads.clear(); changeListeners.forEach((listener) => listener(["volumes", "settings", "schedule"])); }
  const backups = createBackups({
    baseUrl,
    client,
    capability: getCapability,
    generation: () => identityGeneration,
    hosts: listHosts,
    changed: backupChanged,
    admit,
    accepted: (host, id) => {
      refreshHost(host);
      targets.notify(host, id, undefined, true);
    },
  });
  const volumeReads = createVolumeReads(baseUrl, fetcher, listHosts, requireCapability, observeHost, () => identityGeneration, backups.decorate);
  const authoring = createStackAuthoring({
    baseUrl, client, capability: getCapability,
    up: (stackId) => runtime.runStackCommand(stackId, "start"),
    isUser: () => principal?.isAdmin === false,
    labels: async (stackId) => (await (principal?.isAdmin === false ? userInventory : inventory).getStack(stackId)).labels,
    changed: () => changeListeners.forEach((listener) => listener(["stacks"])),
    unauthorized: () => { suspendIdentity(); actions.reset(true); },
  });
  const unsupported = createDisconnectedAdapter();
  const methods = Object.fromEntries(Object.keys(unsupported).filter((key) => key !== "kind").map((operationId) => [operationId, (...args: unknown[]) => Promise.reject(unavailable(operationId, refFor(operationId, args), "capability.mapping-unavailable"))])) as unknown as DockletAdapter;
  const auth = createHttpAuth(baseUrl, client, invalidateIdentity, () => identityGeneration, (user) => { principal = user; });
  const adapter: DockletAdapter = {
    ...methods,
    ...auth,
    async logout() {
      // Taken before admission closes: a read still bootstrapping the token may need its retry to finish it.
      await client.token();
      let settle!: (out: boolean) => void;
      closing = new Promise((resolve) => { settle = resolve; });
      pauseStream();
      // The hub answers reads still in flight with 401 once the session is gone, so they land first (bounded).
      let drained: ReturnType<typeof setTimeout> | undefined;
      await Promise.race([Promise.all([...answering]), new Promise((resolve) => { drained = setTimeout(resolve, LOGOUT_DRAIN_MS); })]);
      clearTimeout(drained);
      try {
        await auth.logout();
      } catch (error) {
        // A session already lost meanwhile counts as signed out; only a live session reopens its reads.
        if (authenticated) { closing = undefined; settle(false); connect(); throw error; }
      }
      signedOut = true;
      closing = undefined;
      settle(true);
    },
    ...createHttpUsers(baseUrl, client),
    ...createHostCommands(baseUrl, client, requireCapability, () => {
      backups.invalidate();
      invalidateCapabilities();
      invalidateEnvironments();
      changeListeners.forEach((listener) => listener(["hosts", ...inventoryTopics]));
    }, getCapability, () => identityGeneration),
    ...createHostChecks({ baseUrl, client, capability: getCapability, generation: () => identityGeneration, changed: () => { invalidateCapabilities(); invalidateEnvironments(); actions.invalidate(true); changeListeners.forEach((listener) => listener(["hosts"])); } }),
    ...createStackUpdates({ baseUrl, client, capability: getCapability, generation: () => identityGeneration, changed: () => { sharedReads.clear(); changeListeners.forEach((listener) => listener(["stacks", "containers", "images", "schedule"])); } }),
    listSystemComponents: createSystemComponents({ baseUrl, client, hosts: listHosts, capability: getCapability, generation: () => identityGeneration }),
    restartAgent: restarts.restartAgent,
    selfUpdate: selfUpdates.selfUpdate,
    getSelfUpdate: selfUpdates.getSelfUpdate,
    installStackUpdates: runtime.installStackUpdates,
    installContainerUpdate: runtime.installContainerUpdate,
    restoreContainerImage: runtime.restoreContainerImage,
    restoreRestorePoint: runtime.restoreRestorePoint,
    admitRuntimeCommand: runtime.admitRuntimeCommand,
    runStackCommand: runtime.runStackCommand,
    runContainerCommand: runtime.runContainerCommand,
    recreateContainer: runtime.recreateContainer,
    restartService: runtime.restartService,
    getRuntimeRun: runtime.getRuntimeRun,
    cancelRuntimeRun: runtime.cancelRuntimeRun,
    releaseRuntimeRun: runtime.releaseRuntimeRun,
    ...authoring,
    ...createContainerOrigin({ baseUrl, client, capability: getCapability, generation: () => identityGeneration, changed: () => { sharedReads.clear(); invalidateCapabilities(); changeListeners.forEach((listener) => listener(["containers", "stacks"])); } }),
    ...createSettingsCore(baseUrl, client),
    ...createUpdateSettings({ baseUrl, client, capability: getCapability, generation: () => identityGeneration }),
    ...createNotificationSettings({ baseUrl, client, capability: getCapability, generation: () => identityGeneration }),
    ...hubBackups,
    // The page's detailed status read is also the browser-wide maintenance observation of this identity.
    async getHubBackupMaintenance() {
      const version = identityGeneration;
      const status = await hubBackups.getHubBackupMaintenance();
      if (version === identityGeneration) maintenance.observe(status);
      return status;
    },
    ...createHttpHistory(baseUrl, client),
    ...createHttpUiPreferences(baseUrl, client, () => ownedWrites.signal),
    ...createHttpServiceWidgets(baseUrl, client),
    kind: "http",
    mappedOperations: IMPLEMENTED,
    getCapability,
    listHosts,
    ...inventory,
    listStacks: (...args) => principal?.isAdmin === false ? userInventory.listStacks(...args) : inventory.listStacks(...args),
    getStack: (id) => principal?.isAdmin === false ? userInventory.getStack(id) : inventory.getStack(id),
    ...resourceReads,
    ...createVulnerabilityCandidates({ baseUrl, client, generation: () => identityGeneration, registration: () => environmentState, capability: getCapability, writeSignal: () => ownedWrites.signal, accepted: (host, id) => { refreshHost(host); targets.notify(host, id, undefined, true); } }),
    ...createVulnerabilityAdvisories({ baseUrl, client, generation: () => identityGeneration, registration: () => environmentState, capability: getCapability, hosts: listHosts, changed: securityChanged }),
    ...createVulnerabilities({ baseUrl, client, generation: () => identityGeneration, capability: getCapability, hosts: listHosts, images: createObservedHostImageReader(baseUrl, fetcher, () => {}), changed: securityChanged, accepted: (host, id) => { refreshHost(host); targets.notify(host, id, undefined, true); } }),
    ...volumeReads,
    ...backups.api,
    async getVolume(id, parent) {
      return inventoryReadBudget(parent)(async (signal) => {
        let volume;
        try { volume = await volumeReads.getVolume(id, signal); }
        catch (error) {
          const missing = error instanceof AdapterOutcomeError
            && error.outcome.kind === "error" && error.outcome.status === 404
            || error instanceof Error && error.message === "not-found";
          if (!missing) throw error;
          const host = id.split("~")[0]!;
          // A stored bind key stays reachable without a current mount (K3); a volume only once it is gone.
          const observed = (await backups.readOverview(host, signal)).volumes.find((v) => v.id === id && (v.kind === "bind" || v.volumeExists === false));
          if (!observed) throw error;
          volume = { ...observed, backups: [] };
        }
        const known = volume.kind === "volume" || !!volume.bind?.eligible;
        const [decorated, early] = await Promise.all([backups.decorate(volume.hostId, [volume], signal), known ? backups.history(id, signal).catch(() => null) : undefined]);
        const entry = { ...volume, ...decorated[0] };
        // Other binds read history only when the overview shows a stored plan or backup (K3/K4).
        if (!known && backupAccess(entry) === "none") return entry;
        const history = known ? early : await backups.history(id, signal).catch(() => null);
        signal.throwIfAborted();
        return { ...entry, backups: history ?? [], backupUnavailable: !history || decorated[0]?.backupUnavailable };
      });
    },
    ...createImageMetadata(baseUrl, fetcher), ...createStackImport(baseUrl, client, () => lifetimeGeneration, getCapability),
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
        try {
          return (await backups.api.getBackupOverview(host)).volumes
            .filter((v) => v.backup?.enabled && v.backup.nextRunAt)
            .map((v) => ({
              id: `${host}~backup-${v.name}`,
              hostId: host,
              kind: "backup" as const,
              title: v.name,
              volumeId: v.id,
              cadence: v.backup!.schedule.mode,
              nextRunAt: v.backup!.nextRunAt,
              enabled: true,
            }));
        }
        // Maintenance fails the whole read, so the query keeps its cached rows while it retries.
        catch (error) { if (isMaintenanceError(error)) throw error; return []; }
      }))).flat();
      return [...backupRows, ...(await Promise.all(selected.map(async (hostId) => {
        try {
          const config = await adapter.listUpdatePolicies!(hostId);
          const timeZone = (await adapter.getUpdateSettings!()).timeZone;
          return config.filter((p) => p.mode === "scheduled" && p.window).map((p) => {
            const window = p.window!;
            const schedule = window.kind === "daily" ? { mode: "daily" as const, time: window.time } : { mode: "weekly" as const, day: (["sun", "mon", "tue", "wed", "thu", "fri", "sat"] as const)[window.day!]!, time: window.time };
            return { id: `${hostId}~${p.targetKind}-${p.targetId}`, hostId, kind: "auto-update" as const, title: p.targetName, cadence: window.kind, nextRunAt: nextOccurrence(schedule, new Date(), timeZone)?.toISOString(), enabled: true, code: p.applyCode };
          });
        } catch (error) {
          if (isCancelledError(error) || isMaintenanceError(error)) throw error;
          return [{ id: `${hostId}~schedule-unavailable`, hostId, kind: "update-check" as const, title: "", cadence: "", unavailable: true }];
        }
      }))).flat()];
    },
    listActions: () => principal?.isAdmin === false ? Promise.resolve(runtime.retained()) : actions.load(),
    observeActionDetail(host, id, listener) {
      const offActions = adapter.subscribeActions(() => {});
      const owner = lifetimeGeneration;
      let active = true;
      const pollUntil = Date.now() + 30 * 60_000;
      let scanTimer: ReturnType<typeof setInterval> | undefined;
      const stopPolling = () => { clearInterval(scanTimer); scanTimer = undefined; };
      const pollScan = (run?: CodedActionRun) => {
        if (!run || (!["scan", "backup", "volume-restore"].includes(run.kind) && run.sourceOperation !== "vulnerability.candidate-scan")
          || !["running", "queued"].includes(run.status)) {
          stopPolling();
          return;
        }
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
    listDashboardEvents: dashboardEvents.load,
    subscribeActions(listener) {
      actionListeners.add(listener);
      runtime.observe(true);
      connect();
      if (actionListeners.size === 1 && sourceOpen) refreshRuns();
      else if (actions.hasSnapshot()) listener(actions.runs());
      return () => { if (!actionListeners.delete(listener)) return; if (!actionListeners.size) { targets.dropIdle(); runtime.observe(false); } disconnectIfUnused(); };
    },
    subscribeStackTerminals(listener) { stackListeners.add(listener); return () => { stackListeners.delete(listener); }; },
    maintenance: { state: maintenance.state, subscribe: maintenance.subscribe },
    subscribeChanges(listener) {
      changeListeners.add(listener);
      connect();
      return () => { changeListeners.delete(listener); disconnectIfUnused(); };
    },
    subscribeMetrics: (listener) => principal?.isAdmin === false ? (() => {}) : metrics(listener),
    subscribeLogs: logs.subscribeLogs,
    subscribeContainerLogs: logs.subscribeContainerLogs,
    openContainerShell: (...args) => { invalidateCapabilities(); return shells.open(...args); },
  };
  registerIdentityLifetime(adapter.subscribeChanges, {
    version: () => lifetimeGeneration,
    active: () => authenticated,
    listen: (notify) => { identityListeners.add(notify); return () => { identityListeners.delete(notify); }; },
  });
  // Concurrent `listActions` callers share one flight; they also share its converted outcome.
  const outcomes = new WeakMap<Promise<unknown>, Promise<unknown>>();
  function sharedOutcome(flight: Promise<unknown>, convert: (error: unknown) => never): Promise<unknown> {
    let outcome = outcomes.get(flight);
    if (!outcome) { outcome = flight.catch(convert); outcomes.set(flight, outcome); }
    return outcome;
  }
  // Guard public reads and partial publishers in addition to transport bodies.
  return new Proxy(adapter, { get(target, key, receiver) {
    const method: unknown = Reflect.get(target, key, receiver);
    if (typeof method !== "function") return method;
    // After a sign-out, a refused non-auth read ends as cancellation whatever error its reader mapped the refusal to.
    const signedOutCancels = (error: unknown) => { throw signedOut && !AUTH_READS.has(String(key)) ? new CancelledError() : error; };
    // File and archive reads keep their own lifetimes; only their sign-out outcome is aligned.
    if (/^(read|browse|download)/.test(String(key))) return (...args: unknown[]) => (method(...args) as Promise<unknown>).catch(signedOutCancels);
    if (!/^(list|get|discover|planVolumePrune)/.test(String(key))) return method;
    return (...args: unknown[]) => {
      // Auth and activities already implement their specialized renewal re-read.
      if (key === "getCurrentUser" || key === "getAuthMode") return method(...args);
      if (key === "listActions") return sharedOutcome(method(...args) as Promise<unknown>, signedOutCancels);
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
      return invoke().catch(signedOutCancels);
    };
  } });
}
