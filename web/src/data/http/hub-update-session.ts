// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original tab-owned hub update observer; identity suspension preserves public update progress.

import { CancelledError } from "@tanstack/react-query";
import type { components } from "@/api/generated/schema";
import { sameProductVersion, validSelfUpdateCheck, type SelfUpdateCheck, type SelfUpdateResponse } from "@/domain/self-update";
import { SELF_UPDATE_CODE_KEYS } from "../wire-catalogue-self-update";
import { AdapterOutcomeError } from "../capability";
import type { CodedActionRun } from "../message-code";
import { createStore } from "@/shared/lib/store";
import { discardResponse } from "./identity-reads";
import type { CsrfClient } from "./csrf-client";
import { object, runtimeError } from "./runtime-mapping";

type Operation = components["schemas"]["SystemSelfUpdateOperation"];
export type HubUpdateState = { phase: "idle" }
  | { phase: "starting"; target: string; startedAt: number }
  | { phase: "running"; target: string; startedAt: number; opId: string; runId?: string; online: boolean; targetSeen: boolean; authenticated: boolean }
  | { phase: "done"; target: string }
  | { phase: "failed"; target: string; runId?: string; code?: string; authenticated: boolean }
  | { phase: "unknown"; target: string; runId?: string; versionSeen?: string; authenticated: boolean }
  | { phase: "signin"; target: string; versionSeen?: string };
export interface HubUpdateSnapshot { state: HubUpdateState; dismissed: boolean }
export const HUB_UPDATE_RECORD = "docklet.hub-update";
interface RecordValue { hubOrigin: string; principalId: string; opId: string; runId?: string; startedAt: number }
interface Options {
  baseUrl: string;
  client: CsrfClient;
  publicFetch: typeof fetch;
  readRun: (host: string, id: string, signal: AbortSignal) => Promise<CodedActionRun>;
  origin?: string;
  storage?: Pick<Storage, "getItem" | "setItem" | "removeItem">;
}
export type HubUpdateSession = ReturnType<typeof createHubUpdateSession>;

export function createHubUpdateSession({ baseUrl, client, publicFetch, readRun, origin = globalThis.location?.origin ?? "", storage }: Options) {
  const store = createStore<HubUpdateSnapshot>({ state: { phase: "idle" }, dismissed: false });
  let lastVersion: string | undefined, runCode: string | undefined;
  let administrator = false;
  let epoch = 0, principalId: string | undefined, baseline: string | undefined, terminalSamples = 0;
  let timer: ReturnType<typeof setTimeout> | undefined, recoveryTimer: ReturnType<typeof setTimeout> | undefined;
  let recoveryEpoch = 0;
  let cancelDelay: (() => void) | undefined;
  const controllers = new Set<AbortController>();
  const local = () => { try { return storage ?? globalThis.sessionStorage; } catch { return undefined; } };
  const removeRecord = () => { try { local()?.removeItem(HUB_UPDATE_RECORD); } catch { /* Storage is optional. */ } };
  function record(): RecordValue | undefined {
    try {
      const raw = local()?.getItem(HUB_UPDATE_RECORD);
      if (!raw) return;
      const row = JSON.parse(raw) as RecordValue;
      if (!row || row.hubOrigin !== origin || row.principalId !== principalId || typeof row.opId !== "string" || !row.opId
        || (row.runId !== undefined && typeof row.runId !== "string") || !Number.isFinite(row.startedAt)
        || row.startedAt > Date.now() || Date.now() - row.startedAt > 2 * 60 * 60_000) { removeRecord(); return; }
      return row;
    } catch { removeRecord(); return; }
  }
  function save(state: Extract<HubUpdateState, { phase: "running" }>) {
    if (!principalId) return;
    try { local()?.setItem(HUB_UPDATE_RECORD, JSON.stringify({ hubOrigin: origin, principalId, opId: state.opId, runId: state.runId, startedAt: state.startedAt })); } catch { /* In-memory progress remains available. */ }
  }
  function stop() { cancelDelay?.(); cancelDelay = undefined; clearTimeout(timer); timer = undefined; controllers.forEach(c => c.abort()); controllers.clear(); }
  function publish(state: HubUpdateState) {
    const terminal = state.phase === "done" || state.phase === "failed" || state.phase === "unknown";
    if (terminal) { removeRecord(); stop(); }
    store.set({ state, dismissed: terminal ? false : store.get().dismissed });
  }
  // Race the whole read, including its body. Abort alone cannot bound a stalled or non-cooperative reader.
  async function bounded<T>(read: (signal: AbortSignal) => Promise<T>, milliseconds = 5000): Promise<T> {
    const controller = new AbortController(); controllers.add(controller);
    let timeout: ReturnType<typeof setTimeout> | undefined;
    const expired = new Promise<never>((_, reject) => {
      controller.signal.addEventListener("abort", () => reject(new CancelledError()), { once: true });
      timeout = setTimeout(() => controller.abort(), milliseconds);
    });
    try { return await Promise.race([read(controller.signal), expired]); }
    finally { clearTimeout(timeout); controllers.delete(controller); }
  }
  async function readJson(path: string, fetcher: typeof fetch, signal: AbortSignal): Promise<unknown> {
    const response = await fetcher(`${baseUrl}${path}`, { credentials: "same-origin", cache: "no-store", signal, headers: { Accept: "application/json" } });
    if (!response.ok) { discardResponse(response); throw runtimeError("http.error", response.status); }
    return response.json();
  }
  const operation = (milliseconds = 5000) => bounded(async signal => {
    const check = await readJson("/environments/0/system/upgrade/check", client.fetch, signal) as SelfUpdateCheck;
    if (!validSelfUpdateCheck(check) || check.operation?.phase === "settled" && !check.operation.outcome) throw runtimeError("update.check-unavailable");
    baseline = check.operation?.opId; lastVersion = check.version;
    return check.operation;
  }, milliseconds);
  const version = () => bounded(async signal => {
    const row = object(await readJson("/app-version", publicFetch, signal));
    if (typeof row.currentVersion !== "string") throw runtimeError("http.invalid-response");
    return row.currentVersion;
  });
  function schedule() {
    clearTimeout(timer);
    if (["running", "signin"].includes(store.get().state.phase)) timer = setTimeout(() => { void poll(); }, 3000);
  }
  function settle(op: Operation, state: Extract<HubUpdateState, { phase: "running" }>) {
    if (op.phase !== "settled") return false;
    if (op.outcome === "healthy") publish({ phase: "done", target: state.target });
    else if (op.outcome === "failed-health" || op.outcome === "failed-pre-effect") publish({ phase: "failed", target: state.target, runId: state.runId, code: op.code, authenticated: true });
    else publish({ phase: "unknown", target: state.target, runId: state.runId, versionSeen: lastVersion, authenticated: true });
    return true;
  }
  function adopt(op: Operation, startedAt: number) {
    stop(); ++epoch; terminalSamples = 0; runCode = undefined;
    const state: Extract<HubUpdateState, { phase: "running" }> = { phase: "running", target: op.targetVersion, opId: op.opId, runId: op.runId, startedAt, online: true, targetSeen: false, authenticated: true };
    store.set({ state, dismissed: false }); save(state);
    if (!settle(op, state)) void poll();
  }
  async function poll() {
    const state = store.get().state, owner = epoch;
    if (state.phase === "signin") {
      try { const v = await version(); if (owner === epoch && store.get().state.phase === "signin") publish({ ...state, versionSeen: v }); } catch { /* Keep the sign-in view during the outage. */ }
      if (owner === epoch) schedule(); return;
    }
    if (state.phase !== "running") return;
    let op: Operation | undefined, opReadable = false, v: string | undefined, run: CodedActionRun | undefined;
    const current = () => owner === epoch && store.get().state.phase === "running";
    // A correlated settled operation decides immediately, without waiting for a slow public version or run read.
    await Promise.allSettled([
      operation().then(value => { op = value; opReadable = true; if (current() && op?.opId === state.opId && sameProductVersion(op.targetVersion, state.target)) settle(op, state); }),
      version().then(value => { v = value; }),
      state.runId ? bounded(signal => readRun("0", state.runId!, signal)).then(value => { if (value.id === state.runId && value.hostId === "0") run = value; }) : Promise.resolve(),
    ]);
    if (!current()) return;
    const correlated = op?.opId === state.opId && sameProductVersion(op.targetVersion, state.target);
    const terminal = !!run && ["succeeded", "failed", "cancelled"].includes(run.status);
    // Adjacent reads may straddle settlement: give the operation two more samples before falling back to the run.
    if (run && terminal && (!opReadable || !correlated || correlated && ++terminalSamples > 2)) {
      if (run.status === "succeeded") publish({ phase: "done", target: state.target });
      else {
        const mapped = runCode ?? run.error?.code ?? run.log?.at(-1)?.code;
        const code = Object.entries(SELF_UPDATE_CODE_KEYS).find(([, key]) => key === mapped)?.[0] ?? mapped;
        publish(code === "update.outcome-unknown" ? { phase: "unknown", target: state.target, runId: state.runId, authenticated: true }
          : { phase: "failed", target: state.target, runId: state.runId, code, authenticated: true });
      }
      return;
    }
    if (!terminal) terminalSamples = 0;
    const next = { ...state, runId: correlated ? op?.runId ?? state.runId : state.runId, online: opReadable || !!v || !!run, authenticated: opReadable || !!run, targetSeen: state.targetSeen || sameProductVersion(v, state.target) };
    publish(next); save(next); schedule();
  }
  function clear() {
    ++epoch; ++recoveryEpoch; clearTimeout(recoveryTimer); stop(); removeRecord(); baseline = undefined;
    store.set({ state: { phase: "idle" }, dismissed: false });
  }
  return {
    ...store,
    clear,
    establish(id: string, admin = true) { if (principalId !== undefined && (principalId !== id || !admin)) clear(); principalId = id; administrator = admin; },
    unauthorized() {
      // A rejected session ends recovery retries; each retry would reject again and could cancel a sign-in.
      clearTimeout(recoveryTimer); ++recoveryEpoch;
      const state = store.get().state;
      if (state.phase !== "running" && state.phase !== "starting") return;
      ++epoch; stop(); removeRecord(); publish({ phase: "signin", target: state.target }); void poll();
    },
    observeRun(id: string, code?: string) {
      const state = store.get().state;
      if (state.phase === "running" && state.runId === id) runCode = code;
    },
    observe(op?: Operation) { baseline = op?.opId; },
    dismiss() { store.set({ ...store.get(), dismissed: true }); },
    async recover() {
      if (!principalId || !administrator || store.get().state.phase !== "idle") return;
      const owner = epoch, recovery = ++recoveryEpoch, startedAt = Date.now(); let attempt = 0;
      clearTimeout(recoveryTimer);
      const read = async () => {
        try {
          const op = await operation();
          if (owner !== epoch || recovery !== recoveryEpoch || store.get().state.phase !== "idle") return;
          const stored = record();
          if (op && (op.phase !== "settled" || stored?.opId === op.opId)) adopt(op, stored?.opId === op.opId ? stored.startedAt : Date.now());
          else removeRecord();
        } catch {
          if (owner !== epoch || recovery !== recoveryEpoch) return;
          const delay = [5000, 10_000, 20_000, 30_000][Math.min(attempt++, 3)]!;
          if (Date.now() + delay < startedAt + 120_000) recoveryTimer = setTimeout(() => { void read(); }, delay);
        }
      };
      await read();
    },
    async start(target: string, submit: () => Promise<SelfUpdateResponse>): Promise<string> {
      if (["starting", "running", "signin"].includes(store.get().state.phase)) throw runtimeError("host.busy");
      const previous = baseline, startedAt = Date.now(), owner = ++epoch;
      clearTimeout(recoveryTimer); ++recoveryEpoch;
      store.set({ state: { phase: "starting", target, startedAt }, dismissed: false });
      try {
        const response = await submit();
        if (owner !== epoch) throw new CancelledError();
        // Admission has no outcome field; only the correlated observation can settle it.
        adopt({ ...response, phase: "reserved", targetVersion: target, helperRunning: false }, startedAt);
        return response.runId ?? "";
      } catch (error) {
        if (owner !== epoch) throw new CancelledError();
        const ambiguous = error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.code === "update.outcome-unknown";
        if (ambiguous) {
          const until = Date.now() + 60_000;
          while (owner === epoch && Date.now() < until) {
            try {
              const op = await operation(Math.min(5000, until - Date.now()));
              if (owner !== epoch) throw new CancelledError();
              if (op && op.opId !== previous && sameProductVersion(op.targetVersion, target)) { adopt(op, startedAt); return op.runId ?? ""; }
            } catch { /* Read-only reconciliation never repeats the POST. */ }
            if (owner !== epoch) throw new CancelledError();
            await new Promise<void>(resolve => { cancelDelay = resolve; timer = setTimeout(() => { cancelDelay = undefined; resolve(); }, Math.min(3000, Math.max(0, until - Date.now()))); });
          }
        }
        if (owner !== epoch) throw new CancelledError();
        publish({ phase: "idle" }); throw error;
      }
    },
  };
}
