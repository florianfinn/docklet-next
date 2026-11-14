// Source: docklet-next/issues/428
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original identity-owned inventory scope retention and automatic read lifetimes. M26 #462: retained combined shell totals. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags.

import { CancelledError } from "@tanstack/react-query";
import type { Volume } from "@/domain/volumes";
import type { Host, InventoryObservation } from "@/domain/model";
import { AdapterOutcomeError } from "../capability";
import { copyInventoryRecovery } from "../inventory-recovery";
import { hostReadDeadline } from "./host-read-deadline";
import { READ_TAG_HEADER, SKIPPED_HEADER, skipReason, type ReadTag } from "./host-read-policy";
import { freshFrom } from "../refresh-owner";
import { isMaintenanceError } from "../maintenance-retry";

const automatic = new Set(["listStacks", "listContainers", "listNavInventory", "listImages", "listVolumes", "listNetworkSummaries", "getTopology"]);
type Observe = (host: string, state: InventoryObservation) => void;
type Catalogue = () => Promise<Host[]>;
type Require = (operation: string) => Promise<void>;
type Factory<T> = (observe: Observe, fetcher: typeof fetch, hosts: Catalogue, require: Require) => T;
type StoredResponse = { body: string; headers: Headers };

/** One adapter identity owns catalogues and automatic snapshots, outside the per-call readers. */
export function createInventoryOwner(generation: () => number, hosts: Catalogue, requireCapability: Require, fetcher: typeof fetch, observe: Observe) {
  let catalogues = new Map<string, Host[]>();
  let responses = new Map<string, StoredResponse>();
  let readHosts = new Map<string, Set<string>>();
  let volumeRows = new Map<string, Volume[]>();
  const current = (version: number) => { if (version !== generation()) throw new CancelledError(); };
  async function scope(operation: string, parent: AbortSignal | undefined, version: number, attempt = parent) {
    try {
      const rows = await hostReadDeadline(30_000, "http.error")(async (signal) => {
        await requireCapability(operation === "listNetworkSummaries" ? "getTopology" : operation === "listNavInventory" ? "listVolumes" : operation); signal.throwIfAborted();
        // The catalogue read starts after every change notified so far: the attempt reads their state.
        if (operation === "listNavInventory") await requireCapability("listContainers");
        freshFrom(attempt);
        const rows = await hosts(); signal.throwIfAborted(); return rows;
      }, parent);
      current(version);
      catalogues.set(operation, structuredClone(rows));
      return { rows: structuredClone(rows), stale: false };
    } catch (error) {
      current(version); parent?.throwIfAborted();
      // Hub maintenance decides nothing about the scope: the query keeps its data and retries (S2 rule B).
      if (isMaintenanceError(error)) throw error;
      const outcome = error instanceof AdapterOutcomeError ? error.outcome : undefined;
      // A transient capability read arrives as not-available with its own reason; denials keep other reasons.
      const transient = error instanceof TypeError || outcome?.kind === "not-available" && outcome.reasonCode === "capability.read-unavailable"
        || outcome?.kind === "error" && (outcome.status !== undefined ? outcome.status >= 500 : ["http.error", "agent.unavailable", "capability.read-unavailable"].includes(outcome.code));
      const retained = catalogues.get(operation);
      if (!transient || !retained) { catalogues.delete(operation); throw error; }
      return { rows: structuredClone(retained), stale: true };
    }
  }
  function automaticFetch(rows: Host[], version: number, loadedHosts: ReadonlySet<string>, tag: ReadTag = "background"): typeof fetch {
    const store = responses;
    return async (input, init) => {
      current(version);
      const key = String(input), previous = store.get(key);
      const host = /\/environments\/([^/]+)\//.exec(key)?.[1];
      const status = rows.find((h) => h.id === decodeURIComponent(host ?? ""))?.status;
      // First offline reads reach hub-retained snapshots; only refreshes may skip those rows.
      const firstOffline = !loadedHosts.has(decodeURIComponent(host ?? "")) && status && !["online", "standby"].includes(status);
      const headers = new Headers(init?.headers);
      if (!firstOffline) headers.set(READ_TAG_HEADER, tag);
      let response = await fetcher(input, { ...init, headers });
      // A catalogue may turn offline while the first read is queued. Its retained hub snapshot still belongs to
      // this first load; retry a synthetic skip once deliberately, before any snapshot exists for this reader.
      if (response.headers.has(SKIPPED_HEADER) && !loadedHosts.has(decodeURIComponent(host ?? ""))) {
        await response.body?.cancel(); current(version); init?.signal?.throwIfAborted();
        headers.delete(READ_TAG_HEADER);
        response = await fetcher(input, { ...init, headers });
      }
      current(version); init?.signal?.throwIfAborted();
      if (response.headers.has(SKIPPED_HEADER) && previous) {
        await response.body?.cancel();
        // The skip keeps the catalogue's persistent refusal as the visible reason.
        const headers = new Headers(previous.headers); headers.set("X-Docklet-Host-Reason", skipReason(status));
        return new Response(previous.body, { status: 200, headers });
      }
      if (response.ok) {
        const body = await response.text(); current(version); init?.signal?.throwIfAborted();
        store.set(key, { body, headers: new Headers(response.headers) });
        return new Response(body, { status: response.status, statusText: response.statusText, headers: response.headers });
      }
      return response;
    };
  }
  function annotate<T>(value: T): T {
    if (!value || typeof value !== "object") return value;
    if (Array.isArray(value)) return value.map(annotate) as T;
    const object = value as Record<string, unknown>;
    const clone = Object.fromEntries(Object.entries(object).map(([key, child]) => [key, annotate(child)]));
    if (typeof object.stale === "boolean") Object.assign(clone, { stale: true, scopeStale: true, recovery: "agent.unavailable" });
    return clone as T;
  }
  return {
    reset() { catalogues = new Map(); responses = new Map(); readHosts = new Map(); volumeRows = new Map(); },
    get volumeRows() { return volumeRows; },
    wrap<T extends object>(make: Factory<T>): T {
      return new Proxy(make(() => {}, fetcher, hosts, requireCapability), { get(_target, key) {
        return async (...args: unknown[]) => {
          const version = generation();
          const notify: Observe = (host, state) => { if (version === generation()) observe(host, state); };
          const method = String(key);
          if (!automatic.has(method)) return Reflect.apply(Reflect.get(make(notify, fetcher, hosts, requireCapability), key) as (...args: unknown[]) => unknown, undefined, args);
          const tag = (method === "listNetworkSummaries" ? args[1] : args[2]) as ReadTag | undefined;
          const parent = method === "listNetworkSummaries" ? args[0] : method === "getTopology" ? undefined : args[1];
          const loadedHosts = readHosts.get(method) ?? new Set<string>(); readHosts.set(method, loadedHosts);
          // Topology reads are not aborted; its last argument only names the attempt for the freshness boundary.
          const attempt = method === "getTopology" ? args[3] : parent;
          const { rows, stale } = await scope(method, parent as AbortSignal | undefined, version, attempt as AbortSignal | undefined);
          const mark = <R extends object>(result: R): R => {
            const next = stale ? annotate(result) : result;
            copyInventoryRecovery(result, next, stale ? rows : []);
            return next;
          };
          const onPartial = args[0];
          if (typeof onPartial === "function") args[0] = (value: object) => { current(version); onPartial(mark(value)); };
          const reader = make((host, state) => notify(host, stale ? { ...state, stale: true, scopeStale: true, recovery: "agent.unavailable" } : state), automaticFetch(rows, version, loadedHosts, tag), async () => structuredClone(rows), async () => {});
          const result = await Reflect.apply(Reflect.get(reader, key) as (...args: unknown[]) => Promise<object>, undefined, args) as object;
          current(version);
          rows.forEach((host) => loadedHosts.add(host.id));
          return mark(result);
        };
      } });
    },
  };
}
