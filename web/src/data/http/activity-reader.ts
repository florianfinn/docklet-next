// Source: docklet-next/issues/102
// Revision: dd403606fbbf763798d43f4ee854adcb105e01b1
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original focused activity source isolation and read-health projection; bounded discovery and coalesced full refreshes (#211).

import { CancelledError, isCancelledError } from "@tanstack/react-query";

import type { RuntimeView } from "@/features/activity/runtime-presentation";
import type { components } from "@/api/generated/schema";
import type { ActionReadFailure, ActionReadHealth } from "../adapter";
import { AdapterOutcomeError } from "../capability";
import type { CodedActionRun } from "../message-code";
import { mapActivityDetail } from "./map-dto";

type Activity = components["schemas"]["ActivityActivity"];
type Detail = components["schemas"]["ActivityDetail"];
interface SourceRead {
  hostId: string;
  runs: CodedActionRun[];
  failure?: ActionReadFailure;
}
interface Reads {
  hosts: () => Promise<string[]>;
  page: (hostId: string) => Promise<{ items: Activity[]; incomplete: boolean }>;
  detail: (hostId: string, id: string) => Promise<Detail>;
  publish: (runs: CodedActionRun[] | undefined, health: ActionReadHealth) => void;
  unauthorized: () => void;
  project?: (run: CodedActionRun) => Promise<CodedActionRun>;
  retained?: () => CodedActionRun[];
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
  let pending: { generation: number; dirty: boolean; promise: Promise<CodedActionRun[]> } | undefined;
  let current: CodedActionRun[] = [];
  let initialized = false;
  let health: ActionReadHealth = { failures: [] };
  let sequence = 0;
  let generation = 0;
  let cancellationEpoch = 0;
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
  async function source(hostId: string): Promise<SourceRead> {
    let page: { items: Activity[]; incomplete: boolean };
    try { page = await reads.page(hostId); }
    catch (error) {
      if (isUnauthorized(error)) throw error;
      return { hostId, runs: [], failure: failure(error, hostId) };
    }
    const version = generation;
    const results: { run?: CodedActionRun; failure?: ActionReadFailure }[] = [];
    let next = 0;
    async function readItem(item: Activity) {
      try {
        if (!item || typeof item.id !== "string" || !item.id || item.environmentId !== hostId) invalid();
        const run = mapped(await reads.detail(hostId, item.id), hostId, item.id);
        return { run: reads.project ? await reads.project(run) : run };
      } catch (error) {
        if (isUnauthorized(error)) throw error;
        return { failure: failure(error, hostId) };
      }
    }
    // Each host advances independently; targeted notifications bypass discovery workers.
    await Promise.all(Array.from({ length: Math.min(4, page.items.length) }, async () => {
      while (next < page.items.length) {
        if (version !== generation) throw new CancelledError();
        const index = next++;
        results[index] = await readItem(page.items[index]!);
      }
    }));
    return { hostId, runs: results.flatMap((r) => r.run ? [r.run] : []), failure: results.find((r) => r.failure)?.failure ?? (page.incomplete ? { hostId, reason: "unavailable" } : undefined) };
  }
  async function performLoad(rereadRenewal = true): Promise<CodedActionRun[]> {
    const started = ++sequence;
    const version = generation;
    const epoch = cancellationEpoch;
    try {
      const hosts = new Set(await reads.hosts());
      const batches = await Promise.all([...hosts].map(source));
      if (version !== generation) throw new CancelledError();
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
      // Each caller still receives its own successful snapshot; failed sources retain observations.
      const observed = new Map(current.map((run) => [run.id, run]));
      for (const batch of batches) {
        if (!batch.failure) for (const [id, run] of observed) if (run.hostId === batch.hostId) observed.delete(id);
        for (const run of batch.runs) observed.set(run.id, run);
      }
      for (const [id, run] of observed) if (!hosts.has(run.hostId!)) observed.delete(id);
      const merged = new Map(current.map((run) => [run.id, run]));
      if (started > discoverySequence) {
        discoverySequence = started;
        knownHosts = hosts;
        for (const [id, run] of merged) if (!hosts.has(run.hostId!)) { merged.delete(id); runSequences.set(id, started); }
        for (const hostId of failures.keys()) if (!hosts.has(hostId)) failures.delete(hostId);
      }
      for (const batch of batches) {
        if (!knownHosts.has(batch.hostId) || started <= (fullSequences.get(batch.hostId) ?? 0)) continue;
        const canApply = (id: string) => started > (runSequences.get(id) ?? 0);
        const incoming = new Set(batch.runs.map((run) => run.id));
        for (const run of batch.runs) if (canApply(run.id)) { merged.set(run.id, run); runSequences.set(run.id, started); }
        if (!batch.failure) for (const [id, run] of merged) {
          if (run.hostId === batch.hostId && !incoming.has(id) && canApply(id)) { merged.delete(id); runSequences.set(id, started); }
        }
        fullSequences.set(batch.hostId, started);
        if (started > (healthSequences.get(batch.hostId) ?? 0)) {
          healthSequences.set(batch.hostId, started);
          if (batch.failure) failures.set(batch.hostId, batch.failure); else failures.delete(batch.hostId);
        }
      }
      for (const run of reads.retained?.() ?? []) {
        if (!merged.has(run.id) && (run.status === "running" || run.status === "queued")) merged.set(run.id, run);
        if (!observed.has(run.id) && (run.status === "running" || run.status === "queued")) observed.set(run.id, run);
      }
      initialized = true;
      current = sorted(merged.values()).map((run) => run.outcomeState === undefined ? run :
        { ...run, runtime: { ...(run as RuntimeView<CodedActionRun>).runtime, sourceObserved: true } });
      if (started > globalHealthSequence) { globalHealthSequence = started; globalFailure = undefined; }
      publishHealth();
      return sorted(observed.values());
    } catch (error) {
      if (version !== generation) {
        // One fresh read may replace work invalidated only by a valid renewal.
        if (rereadRenewal && epoch === cancellationEpoch) return performLoad(false);
        throw new CancelledError();
      }
      if (isCancelledError(error)) throw error;
      if (started > globalHealthSequence) {
        globalHealthSequence = started; globalFailure = failure(error);
        if (isUnauthorized(error)) { ++generation; ++cancellationEpoch; initialized = true; current = []; failures.clear(); reads.unauthorized(); globalHealthSequence = started; globalFailure = failure(error); }
        publishHealth();
      }
      throw error;
    }
  }
  function load(): Promise<CodedActionRun[]> {
    if (pending?.generation === generation) return pending.promise;
    const work = { generation, dirty: false, promise: undefined as unknown as Promise<CodedActionRun[]> };
    work.promise = (async () => {
      let result: CodedActionRun[];
      do {
        work.dirty = false;
        result = await performLoad();
      } while (work.dirty && work.generation === generation);
      return result;
    })();
    pending = work;
    const clear = () => { if (pending === work) pending = undefined; };
    void work.promise.then(clear, clear);
    return work.promise;
  }
  function refresh(): Promise<CodedActionRun[]> {
    // A notification during discovery needs one trailing read of post-event state.
    if (pending?.generation === generation) pending.dirty = true;
    return load();
  }
  async function refreshOne(id: string, hostId: string): Promise<void> {
    const started = ++sequence;
    const version = generation;
    try {
      const mappedRun = mapped(await reads.detail(hostId, id), hostId, id);
      const run = reads.project ? await reads.project(mappedRun) : mappedRun;
      if (version !== generation) throw new CancelledError();
      if (started <= Math.max(runSequences.get(id) ?? 0, fullSequences.get(hostId) ?? 0) || !current.some((r) => r.id === id && r.hostId === hostId)) return;
      current = current.map((r) => r.id === id ? run : r);
      runSequences.set(id, started);
      publish();
      if (run.outcomeState === undefined && run.status !== "queued" && run.status !== "running") await refresh();
    } catch (error) {
      if (version !== generation) throw new CancelledError();
      if (isCancelledError(error)) throw error;
      if (started <= Math.max(runSequences.get(id) ?? 0, fullSequences.get(hostId) ?? 0, healthSequences.get(hostId) ?? 0)) return;
      healthSequences.set(hostId, started);
      runSequences.set(id, started);
      if (isUnauthorized(error)) {
        ++generation; ++cancellationEpoch; initialized = true; current = []; failures.clear(); reads.unauthorized();
        globalHealthSequence = started; globalFailure = failure(error); publishHealth();
      }
      else { failures.set(hostId, failure(error, hostId)); publishHealth(); }
    }
  }
  return {
    load, refresh, refreshOne,
    changed() { if (pending?.generation === generation) pending.dirty = true; },
    upsert(run: CodedActionRun) {
      const existing = current.find((row) => row.id === run.id);
      if (existing && existing.hostId !== run.hostId) return;
      runSequences.set(run.id, ++sequence);
      current = sorted([...current.filter((row) => row.id !== run.id), run]);
      initialized = true;
      publish();
    },
    runs: () => current,
    hasSnapshot: () => initialized,
    health: () => health,
    invalidate(renewed = false) { ++generation; if (!renewed) ++cancellationEpoch; },
    // Loss clears current consumers; a new identity waits for its first real observation.
    reset(hasSnapshot = true) {
      const wasInitialized = initialized;
      ++generation; ++cancellationEpoch; initialized = hasSnapshot; current = []; health = { failures: [] }; globalFailure = undefined; knownHosts.clear();
      failures.clear(); fullSequences.clear(); runSequences.clear(); healthSequences.clear();
      if (wasInitialized || hasSnapshot) publish();
    },
  };
}
