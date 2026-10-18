// Source: docklet-next/issues/102
// Revision: dd403606fbbf763798d43f4ee854adcb105e01b1
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original focused activity source isolation and read-health projection.

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
  page: (hostId: string) => Promise<Activity[]>;
  detail: (hostId: string, id: string) => Promise<Detail>;
  publish: (runs: CodedActionRun[], health: ActionReadHealth) => void;
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
    publish();
  }
  async function source(hostId: string): Promise<SourceRead> {
    let items: Activity[];
    try { items = await reads.page(hostId); }
    catch (error) {
      if (isUnauthorized(error)) throw error;
      return { hostId, runs: [], failure: failure(error, hostId) };
    }
    const results = await Promise.all(items.map(async (item) => {
      try {
        if (!item || typeof item.id !== "string" || !item.id || item.environmentId !== hostId) invalid();
        return { run: mapped(await reads.detail(hostId, item.id), hostId, item.id) };
      } catch (error) {
        if (isUnauthorized(error)) throw error;
        return { failure: failure(error, hostId) };
      }
    }));
    return { hostId, runs: results.flatMap((r) => r.run ? [r.run] : []), failure: results.find((r) => r.failure)?.failure };
  }
  async function load(): Promise<CodedActionRun[]> {
    const started = ++sequence;
    const version = generation;
    try {
      const hosts = new Set(await reads.hosts());
      const batches = await Promise.all([...hosts].map(source));
      if (version !== generation) return [];
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
      initialized = true;
      current = sorted(merged.values());
      if (started > globalHealthSequence) { globalHealthSequence = started; globalFailure = undefined; publishHealth(); }
      else publish();
      return sorted(observed.values());
    } catch (error) {
      if (version !== generation) return [];
      if (started > globalHealthSequence) {
        globalHealthSequence = started; globalFailure = failure(error);
        if (isUnauthorized(error)) { ++generation; initialized = true; current = []; failures.clear(); }
        publishHealth();
      }
      throw error;
    }
  }
  async function refreshOne(id: string, hostId: string): Promise<void> {
    const started = ++sequence;
    const version = generation;
    try {
      const run = mapped(await reads.detail(hostId, id), hostId, id);
      if (version !== generation || started <= Math.max(runSequences.get(id) ?? 0, fullSequences.get(hostId) ?? 0) || !current.some((r) => r.id === id && r.hostId === hostId)) return;
      current = current.map((r) => r.id === id ? run : r);
      runSequences.set(id, started);
      publish();
      if (run.status !== "queued" && run.status !== "running") await load();
    } catch (error) {
      if (version !== generation || started <= Math.max(runSequences.get(id) ?? 0, fullSequences.get(hostId) ?? 0, healthSequences.get(hostId) ?? 0)) return;
      healthSequences.set(hostId, started);
      runSequences.set(id, started);
      if (isUnauthorized(error)) {
        ++generation; initialized = true; current = []; failures.clear();
        globalHealthSequence = started; globalFailure = failure(error); publishHealth();
      }
      else { failures.set(hostId, failure(error, hostId)); publishHealth(); }
    }
  }
  return {
    load, refreshOne,
    runs: () => current,
    hasSnapshot: () => initialized,
    health: () => health,
    invalidate() { ++generation; },
    reset() {
      ++generation; initialized = true; current = []; health = { failures: [] }; globalFailure = undefined; knownHosts.clear();
      failures.clear(); fullSequences.clear(); runSequences.clear(); healthSequences.clear();
      publish();
    },
  };
}
