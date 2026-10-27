// Source: docklet-next/issues/102
// Revision: dd403606fbbf763798d43f4ee854adcb105e01b1
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original focused activity source isolation and read-health projection; bounded discovery and shared generation-scoped notification coalescing (#211).

import { CancelledError, isCancelledError } from "@tanstack/react-query";

import type { RuntimeView } from "@/features/activity/runtime-presentation";
import type { components } from "@/api/generated/schema";
import type { ActionReadFailure, ActionReadHealth } from "../adapter";
import { AdapterOutcomeError } from "../capability";
import type { CodedActionRun } from "../message-code";
import { createSingleFlight } from "./single-flight";
import { activityDeadline, activityTimeout } from "./activity-deadline";
import { mapActivityDetail, mapActivitySummary, summaryOnly, detailWithSummary } from "./map-dto";

type Activity = components["schemas"]["ActivityActivitySummary"];
type Detail = components["schemas"]["ActivityDetail"];
interface SourceRead {
  hostId: string;
  started: number;
  credentials: number;
  runs: CodedActionRun[];
  failure?: ActionReadFailure;
  error?: unknown;
}
interface Reads {
  hosts: () => Promise<string[]>;
  hasConsumer?: () => boolean;
  page: (hostId: string, signal?: AbortSignal) => Promise<{ items: Activity[]; incomplete: boolean }>;
  detail: (hostId: string, id: string, signal?: AbortSignal) => Promise<Detail>;
  publish: (runs: CodedActionRun[] | undefined, health: ActionReadHealth) => void;
  unauthorized: () => void;
  project?: (run: CodedActionRun) => Promise<CodedActionRun>;
  retained?: () => CodedActionRun[];
  mergeSummary?: (run: CodedActionRun) => CodedActionRun;
  detailRun?: (host: string, id: string, signal: AbortSignal, expanded: boolean) => Promise<CodedActionRun>;
  changed?: (previous: CodedActionRun[], current: CodedActionRun[], recovered: string[]) => void;
}

function invalid(): never {
  throw new AdapterOutcomeError({ kind: "error", code: "activity.invalid-data" });
}
function isUnauthorized(error: unknown): boolean {
  return error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status === 401;
}
function failure(error: unknown, hostId?: string): ActionReadFailure {
  const outcome = error instanceof AdapterOutcomeError ? error.outcome : undefined;
  const reason = outcome?.kind === "error" && outcome.status === 401 ? "authentication"
    : outcome?.kind === "error" && outcome.status === 403 ? "permission"
    : outcome?.kind === "error" && /invalid|mapping|short-page/.test(outcome.code) ? "invalid-data" : "unavailable";
  return { hostId, reason };
}
function mapped(detail: Detail, hostId: string, id: string): CodedActionRun {
  if (!detail?.activity || detail.activity.id !== id || detail.activity.environmentId !== hostId) invalid();
  try { return mapActivityDetail(detail); } catch { return invalid(); }
}
const sorted = (runs: Iterable<CodedActionRun>) => [...runs].sort((a, b) => Date.parse(b.startedAt) - Date.parse(a.startedAt));

/** Source-local authority and sequence guards, shared by query and stream reads. */
export function createActivityReader(reads: Reads) {
  let current: CodedActionRun[] = [];
  let initialized = false;
  let health: ActionReadHealth = { failures: [] };
  let sequence = 0;
  let generation = 0;
  let renewal = 0;
  let globalHealthSequence = 0;
  let globalFailure: ActionReadFailure | undefined;
  let discoverySequence = 0;
  let knownHosts = new Set<string>();
  const fullSequences = new Map<string, number>();
  const runSequences = new Map<string, number>();
  const healthSequences = new Map<string, number>();
  const failures = new Map<string, ActionReadFailure>();
  const publish = () => reads.publish(current, health);
  function publishHealth() {
    health = { failures: globalFailure ? [globalFailure, ...failures.values()] : [...failures.values()] };
    // Health before the first observation must not initialize action history.
    reads.publish(initialized ? current : undefined, health);
  }
  async function source(hostId: string, signal: AbortSignal): Promise<SourceRead> {
    const version = generation;
    const started = ++sequence;
    const credentials = renewal;
    let page: { items: Activity[]; incomplete: boolean };
    try { page = await reads.page(hostId, signal); }
    catch (error) {
      if (version !== generation || signal.aborted) throw new CancelledError();
      if (credentials !== renewal) return source(hostId, signal);
      if (isUnauthorized(error)) { ++generation; cancelAll(error); initialized = true; current = []; failures.clear(); reads.unauthorized(); globalHealthSequence = ++sequence; globalFailure = failure(error); publishHealth(); throw error; }
      if (isCancelledError(error)) throw error;
      return { hostId, started, credentials, runs: [], failure: failure(error, hostId) };
    }
    if (version !== generation || signal.aborted) throw new CancelledError();
    if (credentials !== renewal) return source(hostId, signal);
    const results = page.items.map((item) => {
      try {
        if (!item || typeof item.id !== "string" || !item.id || item.environmentId !== hostId) invalid();
        return { run: mapActivitySummary(item), failure: undefined };
      } catch { return { run: undefined, failure: { hostId, reason: "invalid-data" as const } }; }
    });
    return { hostId, started, credentials, runs: results.flatMap((r) => r.run ? [r.run] : []), failure: results.find((r) => r.failure)?.failure ?? (page.incomplete ? { hostId, reason: "unavailable" } : undefined) };
  }
  const sourceFlights = new Map<string, ReturnType<typeof createSingleFlight<SourceRead>>>();
  function sourceFlight(host: string) {
    let entry = sourceFlights.get(host);
    if (!entry) {
      entry = createSingleFlight((signal) => activityDeadline((owned) => source(host, owned), signal), () => generation,
        { hasConsumer: reads.hasConsumer ?? (() => true), deadlineMs: 40_000, timeout: activityTimeout });
      sourceFlights.set(host, entry);
    }
    return entry;
  }

  async function performLoad(signal: AbortSignal, scope?: string, prefetched: SourceRead[] = []): Promise<CodedActionRun[]> {
    const started = ++sequence;
    const version = generation;
    const credentials = renewal;
    let completed = prefetched;
    try {
      const hosts = scope ? new Set(knownHosts) : new Set(await activityDeadline(() => reads.hosts(), signal));
      if (scope && !hosts.has(scope)) return current;
      if (version !== generation || credentials !== renewal || signal.aborted) throw new CancelledError();
      if (!scope && started > discoverySequence) {
        discoverySequence = started; knownHosts = hosts;
        for (const [host, entry] of sourceFlights) if (!hosts.has(host)) { entry.cancel(); sourceFlights.delete(host); }
      }
      const fresh = new Map(prefetched.filter((batch) => batch.credentials === credentials).map((batch) => [batch.hostId, batch]));
      const batches = completed = await Promise.all([...(scope ? [scope] : hosts)].map(async (host): Promise<SourceRead> => {
        try { return fresh.get(host) ?? await sourceFlight(host).request(); }
        catch (error) { if (version !== generation || signal.aborted) throw new CancelledError(); if (isUnauthorized(error)) { ++generation; cancelAll(error); initialized = true; current = []; failures.clear(); reads.unauthorized(); globalHealthSequence = ++sequence; globalFailure = failure(error); publishHealth(); throw error; } if (isCancelledError(error)) throw error; return { hostId: host, started, credentials, runs: [], failure: failure(error, host), error }; }
      }));
      if (version !== generation || credentials !== renewal || signal.aborted) throw new CancelledError();
      if (batches.some((batch) => batch.credentials !== renewal)) return performLoad(signal, scope, batches);
      // A raw run UUID belongs to one source. Reject conflicting claims without changing IDs.
      const owners = new Map(current.map((run) => [run.id, run.hostId]));
      const claims = new Map<string, Set<string>>();
      for (const batch of batches) for (const run of batch.runs) {
        const set = claims.get(run.id) ?? new Set<string>();
        set.add(batch.hostId); claims.set(run.id, set);
      }
      for (const batch of batches) batch.runs = batch.runs.filter((run) => {
        if ((claims.get(run.id)?.size ?? 0) <= 1 && (!owners.has(run.id) || owners.get(run.id) === batch.hostId)) return true;
        batch.failure = { hostId: batch.hostId, reason: "invalid-data" };
        return false;
      });
      // Failed sources retain their observations in the shared full snapshot.
      const observed = new Map(current.map((run) => [run.id, run]));
      const previousRuns = new Map(observed);
      const summary = (run: CodedActionRun) => {
        const previous = previousRuns.get(run.id);
        // List summaries have no scan counters; retain only the same source/run observation.
        return summaryOnly(run.kind === "scan" && previous?.kind === "scan" && previous.hostId === run.hostId
          ? { ...run, scanProgress: run.scanProgress ?? previous.scanProgress } : run);
      };
      for (const batch of batches) {
        if (!batch.failure) for (const [id, run] of observed) if (run.hostId === batch.hostId) observed.delete(id);
        for (const run of batch.runs) observed.set(run.id, summary(run));
      }
      for (const [id, run] of observed) if (!hosts.has(run.hostId!)) observed.delete(id);
      const merged = new Map(current.map((run) => [run.id, run]));
      if (!scope && started >= discoverySequence) {
        discoverySequence = started;
        knownHosts = hosts;
        for (const [id, run] of merged) if (!hosts.has(run.hostId!)) { merged.delete(id); runSequences.set(id, started); }
        for (const hostId of failures.keys()) if (!hosts.has(hostId)) failures.delete(hostId);
      }
      const recovered: string[] = [];
      for (const batch of batches) {
        const observedAt = batch.started;
        if (!knownHosts.has(batch.hostId) || observedAt <= (fullSequences.get(batch.hostId) ?? 0)) continue;
        const canApply = (id: string) => observedAt > (runSequences.get(id) ?? 0);
        const incoming = new Set(batch.runs.map((run) => run.id));
        for (const run of batch.runs) if (canApply(run.id)) { merged.set(run.id, summary(reads.mergeSummary?.(run) ?? run)); runSequences.set(run.id, observedAt); }
        if (!batch.failure) for (const [id, run] of merged) {
          if (run.hostId === batch.hostId && !incoming.has(id) && canApply(id)) { merged.delete(id); runSequences.set(id, observedAt); }
        }
        fullSequences.set(batch.hostId, observedAt);
        if (observedAt > (healthSequences.get(batch.hostId) ?? 0)) {
          healthSequences.set(batch.hostId, observedAt);
          if (batch.failure) failures.set(batch.hostId, batch.failure); else { if (failures.has(batch.hostId)) recovered.push(batch.hostId); failures.delete(batch.hostId); }
        }
      }
      for (const run of reads.retained?.() ?? []) {
        if (!merged.has(run.id) && (run.status === "running" || run.status === "queued")) merged.set(run.id, summaryOnly(run));
        if (!observed.has(run.id) && (run.status === "running" || run.status === "queued")) observed.set(run.id, summaryOnly(run));
      }
      initialized = true;
      const previous = current;
      current = sorted(merged.values()).map((run) => run.outcomeState === undefined ? run :
        { ...run, runtime: { ...(run as RuntimeView<CodedActionRun>).runtime, sourceObserved: true } });
      if (started > globalHealthSequence) { globalHealthSequence = started; globalFailure = undefined; }
      publishHealth();
      reads.changed?.(previous, current, recovered);
      const expired = batches.find((batch) => batch.error instanceof AdapterOutcomeError && batch.error.outcome.kind === "error" && batch.error.outcome.code === "activity.read-timeout");
      if (expired) throw expired.error;
      return sorted(observed.values()).map((run) => summaryOnly(reads.mergeSummary?.(run) ?? run));
    } catch (error) {
      if (version !== generation || signal.aborted) throw new CancelledError();
      // Replay membership and superseded sources; keep source results already read after renewal.
      if (credentials !== renewal) return performLoad(signal, scope, completed);
      if (isCancelledError(error)) throw error;
      if (started > globalHealthSequence) {
        globalHealthSequence = started; globalFailure = failure(error);
        if (isUnauthorized(error)) { ++generation; flight.cancel(error); initialized = true; current = []; failures.clear(); reads.unauthorized(); globalHealthSequence = started; globalFailure = failure(error); }
        publishHealth();
      }
      throw error;
    }
  }
  const flight = createSingleFlight(performLoad, () => generation, { hasConsumer: reads.hasConsumer ?? (() => true), deadlineMs: 90_000, lineageMs: 90_000, timeout: activityTimeout });
  const explicitWaiters = new WeakMap<Promise<CodedActionRun[]>, Promise<CodedActionRun[]>>();
  const load = () => {
    const shared = flight.request();
    let bounded = explicitWaiters.get(shared);
    if (!bounded) {
      const owned = activityDeadline(() => shared, undefined, 90_000); explicitWaiters.set(shared, owned);
      void owned.finally(() => { if (explicitWaiters.get(shared) === owned) explicitWaiters.delete(shared); }).catch(() => undefined);
      bounded = owned;
    }
    return bounded;
  };
  const scopedReads = new WeakMap<Promise<SourceRead | undefined>, Promise<unknown>>();
  function refreshHost(host: string): Promise<unknown> {
    if (!(reads.hasConsumer?.() ?? true)) return Promise.resolve();
    if (!knownHosts.has(host)) return flight.notify();
    const sourceRead = sourceFlight(host).notify();
    const existing = scopedReads.get(sourceRead); if (existing) return existing;
    const version = generation;
    const task = (async () => {
      const batch = await sourceRead;
      if (version !== generation) throw new CancelledError();
      if (!batch) return current;
      if (!initialized) await flight.request();
      if (version !== generation) throw new CancelledError();
      return performLoad(new AbortController().signal, host, [batch]);
    })();
    scopedReads.set(sourceRead, task); return task;
  }

  const refresh = (recovery = false) => {
    if (recovery) {
      for (const entry of sourceFlights.values()) entry.recoverActive();
      return flight.recover();
    }
    if (!flight.isActive()) for (const entry of sourceFlights.values()) entry.dirty();
    return flight.notify();
  };
  function cancelAll(reason?: unknown) { flight.cancel(reason); for (const entry of sourceFlights.values()) entry.cancel(reason); }
  async function readDetail(id: string, hostId: string, signal?: AbortSignal, expanded = false): Promise<CodedActionRun> {
    const started = ++sequence;
    const version = generation;
    const credentials = renewal;
    try {
      const run = await activityDeadline(async (owned) => reads.detailRun ? reads.detailRun(hostId, id, owned, expanded) : mapped(await reads.detail(hostId, id, owned), hostId, id), signal);
      if (version !== generation || signal?.aborted) throw new CancelledError();
      if (credentials !== renewal) return readDetail(id, hostId, signal, expanded);
      if (run.id !== id || run.hostId !== hostId) invalid();
      const existing = current.find((row) => row.id === id && row.hostId === hostId);
      if (!existing) return run;
      if (started <= Math.max(runSequences.get(id) ?? 0, fullSequences.get(hostId) ?? 0)) return detailWithSummary(existing, run);
      current = current.map((r) => r.id === id ? { ...r, ...summaryOnly(run) } : r);
      runSequences.set(id, started);
      publish();
      return run;
    } catch (error) {
      if (version !== generation || signal?.aborted) throw new CancelledError();
      if (credentials !== renewal) return readDetail(id, hostId, signal, expanded);
      if (isCancelledError(error)) throw error;
      if (started <= Math.max(runSequences.get(id) ?? 0, fullSequences.get(hostId) ?? 0, healthSequences.get(hostId) ?? 0)) throw error;
      healthSequences.set(hostId, started);
      runSequences.set(id, started);
      if (isUnauthorized(error)) {
        ++generation; flight.cancel(error); initialized = true; current = []; failures.clear(); reads.unauthorized();
        globalHealthSequence = started; globalFailure = failure(error); publishHealth();
      }
      else { failures.set(hostId, failure(error, hostId)); publishHealth(); }
      throw error;
    }
  }
  const refreshOne = async (id: string, hostId: string, discoverOnTerminal = true) => {
    try { const run = await readDetail(id, hostId); if (discoverOnTerminal && run.outcomeState === undefined && run.status !== "queued" && run.status !== "running") await refresh(); }
    catch (error) { if (isCancelledError(error)) throw error; }
  };

  return {
    load, refresh, refreshOne, readDetail, refreshHost,
    markError() { flight.markError(); for (const entry of sourceFlights.values()) entry.markError(); },
    hostError(host: string) { if (!knownHosts.has(host)) return; const started = ++sequence; healthSequences.set(host, started); failures.set(host, { hostId: host, reason: "unavailable" }); publishHealth(); },
    upsert(run: CodedActionRun) {
      const existing = current.find((row) => row.id === run.id);
      if (existing && existing.hostId !== run.hostId) return;
      runSequences.set(run.id, ++sequence);
      current = sorted([...current.filter((row) => row.id !== run.id), summaryOnly(run)]);
      initialized = true;
      publish();
    },
    runs: () => current,
    hasSnapshot: () => initialized,
    hasHost: (host: string) => knownHosts.has(host),
    health: () => health,
    invalidate(renewed = false) {
      if (renewed) ++renewal;
      else { ++generation; cancelAll(); }
    },
    // Loss clears current consumers; a new identity waits for its first real observation.
    reset(hasSnapshot = true) {
      const wasInitialized = initialized;
      ++generation; cancelAll(); initialized = hasSnapshot; current = []; health = { failures: [] }; globalFailure = undefined; knownHosts.clear();
      failures.clear(); fullSequences.clear(); runSequences.clear(); healthSequences.clear();
      if (wasInitialized || hasSnapshot) publish();
    },
  };
}
