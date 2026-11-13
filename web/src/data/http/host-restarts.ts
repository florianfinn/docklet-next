// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original one-shot restart intake and identity-owned durable phase following. M14 host maintenance and availability. M19: total contract-code catalogue maps. M25 latest restart read after a reload.

import { CancelledError } from "@tanstack/react-query";
import type { components } from "@/api/generated/schema";
import type { CodedActionRun, MessageCode } from "../message-code";
import { AdapterOutcomeError, unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import { RESTART_STATE_KEYS } from "../wire-catalogue-states";
import { operationReasonKey } from "../operation-reason";
import type { CsrfClient } from "./csrf-client";
import { runtimeError, runtimeResponseError } from "./runtime-mapping";
import { activeNow, pausableTimeout } from "./deadline-pause";

type Restart = components["schemas"]["EnvironmentAgentRestartRun"];
type Latest = components["schemas"]["EnvironmentLatestAgentRestart"];
interface Options {
  baseUrl: string; client: CsrfClient;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  generation: () => number; publish: (run: CodedActionRun) => void; changed: () => void;
}
const phases = ["reserved", "sent", "awaiting", "connected"] as const;
const terminal = (row: Restart) => ["connected", "failed", "unknown"].includes(row.state);
const message = (code: string): MessageCode => ({ code, params: {} });
// Local follow budget: the producer deadline span plus grace, from now on the active clock.
const budget = (row: Restart) => activeNow() + Math.max(0, Date.parse(row.deadline) - Date.parse(row.createdAt)) + 60_000;
const isoDate = (value: unknown) => typeof value === "string" && Number.isFinite(Date.parse(value));
// Progress order within one run: the phases, then any terminal state.
const rank = (row: Restart) => terminal(row) ? phases.length : phases.indexOf(row.state as typeof phases[number]);
// A run read without its in-session history marks done only the phases its code proves.
const settledReach = (row: Restart) => row.state === "connected" ? 3 : row.code === "agent.restart-timeout" ? 2 : ["host.busy", "agent.restart-unsupported", "agent.restart-unconfirmed"].includes(row.code ?? "") ? 1 : 0;
function validRun(row: Restart | undefined): Restart {
  const done = row !== undefined && terminal(row);
  if (!row || typeof row.runId !== "string" || !row.runId || ![...phases, "failed", "unknown"].includes(row.state) || !isoDate(row.createdAt) || !isoDate(row.deadline) || ((row.state === "failed" || row.state === "unknown") && !row.code) || (row.state === "connected" && row.code) || done !== isoDate(row.settledAt)) throw runtimeError("hosts.invalid-restart");
  return row;
}
export function createHostRestarts({ baseUrl, client, capability, generation, publish, changed }: Options) {
  // reached: highest observed phase; until: local follow budget (producer deadline span plus grace).
  const rows = new Map<string, { host: string; row: Restart; run: CodedActionRun; reached: number; until: number }>();
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  const pending = new Map<string, Promise<string>>();
  const controllers = new Set<AbortController>();
  let epoch = 0;
  function build(host: string, row: Restart, reached: number): CodedActionRun {
    const phase = phases.indexOf(row.state as typeof phases[number]);
    const done = terminal(row);
    // A failed or unknown restart marks done only phases that were observed.
    const failedStep = (index: number) => index < reached ? "done" : index === reached ? "error" : "skipped";
    const reason = operationReasonKey(row.code ?? "");
    const run: CodedActionRun = {
      id: row.runId, hostId: host, kind: "agent-restart", title: message("activity.runKind.agent-restart"),
      target: { code: "runMessages.value", params: { value: host } },
      services: [], status: row.state === "connected" ? "succeeded" : done ? "failed" : "running",
      startedAt: row.createdAt, finishedAt: row.settledAt,
      progress: row.state === "connected" ? 100 : phase < 0 ? 0 : phase * 25,
      steps: phases.map((name, index) => ({ key: ["validate", "stop", "wait", "connect"][index] as "validate" | "stop" | "wait" | "connect", label: message(RESTART_STATE_KEYS[name]), state: row.state === "connected" ? "done" : done ? failedStep(index) : index < phase ? "done" : index === phase ? "running" : "pending" })),
      ...(done && row.code ? { error: message(row.code === "host.busy" ? "runMessages.restartRefused" : reason ? `containers.operationReasons.${reason}` : "hosts.errors.failed") } : {}),
      log: [], output: [],
    };
    return run;
  }
  function retain(host: string, row: Restart) {
    const prior = rows.get(row.runId);
    const reached = Math.max(prior?.reached ?? 0, phases.indexOf(row.state as typeof phases[number]));
    // The follow budget runs on the active clock: a hub maintenance hold does not use it up.
    const until = prior?.until ?? budget(row);
    const run = build(host, row, reached);
    rows.set(row.runId, { host, row, run, reached, until });
    if (rows.size > 128) { const old = [...rows.values()].find(item => terminal(item.row) && item.row.runId !== row.runId); if (old) rows.delete(old.row.runId); }
    publish(run); changed();
    return run;
  }
  async function request<T = Restart>(host: string, suffix: string, method = "GET", signal?: AbortSignal, parse: (data: unknown) => T = (data) => validRun(data as Restart) as T): Promise<T> {
    const owner = generation(), version = epoch;
    const current = () => { if (owner !== generation() || version !== epoch || signal?.aborted) throw new CancelledError(); };
    const controller = new AbortController(); controllers.add(controller);
    const abort = () => controller.abort(); signal?.addEventListener("abort", abort, { once: true });
    const stopTimer = pausableTimeout(15_000, abort);
    try {
      const response = await client.fetch(`${baseUrl}/environments/${encodeURIComponent(host)}/agent/${suffix}`, { method, credentials: "same-origin", cache: "no-store", signal: controller.signal, headers: { Accept: "application/json" } });
      current(); const body = await response.json(); current();
      if (!response.ok) throw runtimeResponseError(body, response.status);
      if (body.success !== true) throw runtimeError("hosts.invalid-restart");
      return parse(body.data);
    } finally { stopTimer(); controllers.delete(controller); signal?.removeEventListener("abort", abort); }
  }
  function schedule(host: string, id: string) {
    if (timers.has(id) || !rows.has(id) || terminal(rows.get(id)!.row)) return;
    const version = epoch, owner = generation();
    timers.set(id, setTimeout(() => {
      timers.delete(id);
      if (version !== epoch || owner !== generation()) return;
      let stop = false;
      // A removed run or host, or any read past the follow budget, keeps the last observation.
      void detail(host, id).catch((error: unknown) => { stop = (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status === 404); })
        .finally(() => { if (!stop && activeNow() <= (rows.get(id)?.until ?? 0) && version === epoch && owner === generation()) schedule(host, id); });
    }, 1000));
  }
  async function detail(host: string, id: string, signal?: AbortSignal): Promise<CodedActionRun> {
    const known = rows.get(id);
    if (!known || known.host !== host) throw unavailable("restartAgent", { kind: "host", id: host });
    if (terminal(known.row)) return known.run;
    const row = await request(host, `restarts/${encodeURIComponent(id)}`, "GET", signal);
    if (row.runId !== id) throw runtimeError("hosts.invalid-restart");
    // A late active snapshot cannot regress a terminal observation.
    if (terminal(rows.get(id)!.row)) return rows.get(id)!.run;
    return retain(host, row);
  }
  return {
    restartAgent(host: string): Promise<string> {
      if (host === "0" || !host || host.includes("~")) return Promise.reject(unavailable("restartAgent", { kind: "host", id: host }));
      const existing = pending.get(host); if (existing) return existing;
      const owner = generation(), version = epoch;
      const task = (async () => {
        const cap = await capability("restartAgent", { kind: "host", id: host });
        if (owner !== generation() || version !== epoch) throw new CancelledError();
        if (!cap.available) throw unavailable("restartAgent", cap.resourceRef, cap.disabledReasonCode);
        // No intake retry: an ambiguous POST may already have restarted the agent.
        const row = await request(host, "restart", "POST"); retain(host, row); schedule(host, row.runId);
        return row.runId;
      })();
      pending.set(host, task);
      void task.finally(() => { if (pending.get(host) === task) pending.delete(host); }).catch(() => {});
      return task;
    },
    detail,
    /** The host's active or newest restart; history is returned unpublished, an active run is followed. */
    async latest(host: string, signal?: AbortSignal): Promise<CodedActionRun | undefined> {
      if (host === "0" || !host || host.includes("~")) return undefined;
      const row = await request(host, "restarts/latest", "GET", signal, (data) => {
        if (!data || typeof data !== "object" || Array.isArray(data)) throw runtimeError("hosts.invalid-restart");
        const run = (data as Latest).run;
        return run === undefined ? undefined : validRun(run);
      });
      if (!row) return undefined;
      // Within one run the further observation wins. A reopen of a still active run grants a fresh
      // follow budget, so a follow that stopped resumes; background following stays bounded.
      const known = rows.get(row.runId);
      if (known?.host === host) {
        const run = rank(row) <= rank(known.row) ? known.run : retain(host, row);
        const current = rows.get(row.runId);
        if (current && !terminal(current.row)) { current.until = budget(row); schedule(host, row.runId); }
        return run;
      }
      if (terminal(row)) return build(host, row, settledReach(row));
      const run = retain(host, row); schedule(host, row.runId);
      return run;
    },
    has: (host: string, id: string) => rows.get(id)?.host === host,
    retained: () => [...rows.values()].map(item => item.run),
    reset() { ++epoch; timers.forEach(clearTimeout); timers.clear(); controllers.forEach(c => c.abort()); controllers.clear(); pending.clear(); rows.clear(); },
  };
}
