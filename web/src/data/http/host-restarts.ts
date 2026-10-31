// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original one-shot restart intake and identity-owned durable phase following.

import { CancelledError } from "@tanstack/react-query";
import type { components } from "@/api/generated/schema";
import type { CodedActionRun, MessageCode } from "../message-code";
import { unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import { operationReasonKey } from "../operation-reason";
import type { CsrfClient } from "./csrf-client";
import { runtimeError, runtimeResponseError } from "./runtime-mapping";

type Restart = components["schemas"]["EnvironmentAgentRestartRun"];
interface Options {
  baseUrl: string; client: CsrfClient;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  generation: () => number; publish: (run: CodedActionRun) => void; changed: () => void;
}
const phases = ["reserved", "sent", "awaiting", "connected"] as const;
const terminal = (row: Restart) => ["connected", "failed", "unknown"].includes(row.state);
const message = (code: string): MessageCode => ({ code, params: {} });
export function createHostRestarts({ baseUrl, client, capability, generation, publish, changed }: Options) {
  const rows = new Map<string, { host: string; row: Restart; run: CodedActionRun }>();
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  const pending = new Map<string, Promise<string>>();
  const controllers = new Set<AbortController>();
  let epoch = 0;
  function retain(host: string, row: Restart) {
    const phase = phases.indexOf(row.state as typeof phases[number]);
    const done = terminal(row);
    const reason = operationReasonKey(row.code ?? "");
    const run: CodedActionRun = {
      id: row.runId, hostId: host, kind: "agent-restart", title: message("activity.runKind.agent-restart"),
      target: { code: "runMessages.value", params: { value: host } },
      services: [], status: row.state === "connected" ? "succeeded" : done ? "failed" : "running",
      startedAt: row.createdAt, finishedAt: row.settledAt,
      progress: row.state === "connected" ? 100 : phase < 0 ? 0 : phase * 25,
      steps: phases.map((name, index) => ({ key: ["validate", "stop", "wait", "connect"][index] as "validate" | "stop" | "wait" | "connect", label: message(`hosts.restart.phases.${name}`), state: row.state === "connected" ? "done" : done ? index === 3 ? "error" : "done" : index < phase ? "done" : index === phase ? "running" : "pending" })),
      ...(done && row.code ? { error: message(reason ? `containers.operationReasons.${reason}` : "hosts.errors.failed") } : {}),
      log: [], output: [],
    };
    rows.set(row.runId, { host, row, run });
    if (rows.size > 128) { const old = [...rows.values()].find(item => terminal(item.row) && item.row.runId !== row.runId); if (old) rows.delete(old.row.runId); }
    publish(run); changed();
    return run;
  }
  async function request(host: string, suffix: string, method = "GET", signal?: AbortSignal): Promise<Restart> {
    const owner = generation(), version = epoch;
    const current = () => { if (owner !== generation() || version !== epoch || signal?.aborted) throw new CancelledError(); };
    const controller = new AbortController(); controllers.add(controller);
    const abort = () => controller.abort(); signal?.addEventListener("abort", abort, { once: true });
    const timeout = setTimeout(abort, 15_000);
    try {
      const response = await client.fetch(`${baseUrl}/environments/${encodeURIComponent(host)}/agent/${suffix}`, { method, credentials: "same-origin", cache: "no-store", signal: controller.signal, headers: { Accept: "application/json" } });
      current(); const body = await response.json(); current();
      if (!response.ok) throw runtimeResponseError(body, response.status);
      const row = body.data as Restart;
      if (body.success !== true || !row?.runId || ![...phases, "failed", "unknown"].includes(row.state) || !Number.isFinite(Date.parse(row.createdAt)) || !Number.isFinite(Date.parse(row.deadline)) || ((row.state === "failed" || row.state === "unknown") && !row.code) || (row.state === "connected" && row.code)) throw runtimeError("hosts.invalid-restart");
      return row;
    } finally { clearTimeout(timeout); controllers.delete(controller); signal?.removeEventListener("abort", abort); }
  }
  function schedule(host: string, id: string) {
    if (timers.has(id) || !rows.has(id) || terminal(rows.get(id)!.row)) return;
    const version = epoch, owner = generation();
    timers.set(id, setTimeout(() => {
      timers.delete(id);
      if (version !== epoch || owner !== generation()) return;
      void detail(host, id).catch(() => {}).finally(() => { if (version === epoch && owner === generation()) schedule(host, id); });
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
    has: (host: string, id: string) => rows.get(id)?.host === host,
    retained: () => [...rows.values()].map(item => item.run),
    reset() { ++epoch; timers.forEach(clearTimeout); timers.clear(); controllers.forEach(c => c.abort()); controllers.clear(); pending.clear(); rows.clear(); },
  };
}
