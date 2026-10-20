// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { CancelledError } from "@tanstack/react-query";
import type { ContainerCommand, RuntimeReleaseInput, StackCommand } from "../adapter";
import { AdapterOutcomeError, unavailable } from "../capability";
import type { CapabilityDescriptor, ResourceRef } from "../capability";
import type { CsrfClient } from "./csrf-client";
import { splitInventoryId } from "./inventory-mapping";
import { mapRuntime, object, runtimeError, runtimeResponseError, type RuntimeDetail, type RuntimeRun } from "./runtime-mapping";

export const COMMAND_RETRY_MS = 10 * 60_000;
const LIMIT = 256;
const stackRoutes: Record<StackCommand, string> = { start: "up", stop: "down", restart: "restart", redeploy: "redeploy" };
const active = (run: RuntimeRun) => run.status === "running" || run.status === "queued";

/** HTTP contexts still have secure random bytes even without Crypto.randomUUID. */
function commandUUID(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6]! & 15) | 64;
  bytes[8] = (bytes[8]! & 63) | 128;
  const hex = Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}


interface Options {
  baseUrl: string;
  client: CsrfClient;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  publish: (run: RuntimeRun) => void;
  unauthorized?: () => void;
  now?: () => number;
  uuid?: () => string;
}

/** Commands own only browser intent and observation. The hub owns admission and effects. */
export function createRuntimeCommands({ baseUrl, client, capability, publish, unauthorized = () => {}, now = () => performance.now(), uuid = commandUUID }: Options) {
  let generation = 0;
  let observed = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let rights: Promise<Record<string, unknown>> | undefined;
  const controllers = new Set<AbortController>();
  const pending = new Map<string, Promise<string>>();
  const ambiguous = new Set<string>();
  const runs = new Map<string, RuntimeRun>();
  const attempts = new Map<string, number>();
  const reads = new Map<string, Promise<RuntimeRun>>();
  const sequences = new Map<string, number>();
  let sequence = 0;
  const check = (version: number) => { if (version !== generation) throw new CancelledError(); };
  const route = (host: string, id: string) => `${baseUrl}/environments/${encodeURIComponent(host)}/runtime-runs/${encodeURIComponent(id)}`;

  function remember(run: RuntimeRun, notify = true) {
    if (!runs.has(run.id) && runs.size >= LIMIT) {
      const finished = [...runs.values()].find((row) => !active(row));
      if (finished) { runs.delete(finished.id); attempts.delete(finished.id); sequences.delete(finished.id); }
      else { sequences.delete(run.id); attempts.delete(run.id); if (notify) publish(run); return run; }
    }
    runs.set(run.id, run);
    if (notify) publish(run);
    schedule();
    return run;
  }
  async function request(url: string, init: RequestInit = {}): Promise<unknown> {
    const version = generation;
    const controller = new AbortController(); controllers.add(controller);
    const timeout = setTimeout(() => controller.abort(), 15_000);
    try {
      const response = await client.fetch(url, { ...init, signal: controller.signal, headers: { Accept: "application/json", ...(init.body ? { "Content-Type": "application/json" } : {}), ...init.headers } });
      check(version);
      let body: unknown;
      try { body = await response.json(); } catch { throw runtimeError("runtime.invalid-response", response.status); }
      check(version);
      if (!response.ok) throw runtimeResponseError(body, response.status);
      if (response.status !== 200 || object(body).success !== true || !object(body).data) throw runtimeError("runtime.invalid-response", response.status);
      return object(body).data;
    } catch (error) {
      if (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status === 401) { unauthorized(); throw error; }
      check(version);
      throw error instanceof AdapterOutcomeError ? error : runtimeError("http.error");
    }
    finally { clearTimeout(timeout); controllers.delete(controller); }
  }
  async function access(host: string) {
    rights ??= request(`${baseUrl}/auth/me`).then(object).catch(() => ({}));
    const user = await rights;
    const permissions = object(user.permissionsByEnv);
    const allowed = (permission: string) => [permissions.global, permissions[host]].some((list) => Array.isArray(list) && list.includes(permission));
    const canCancel = allowed("activities:read") && allowed("activities:cancel");
    return { canCancel, canRelease: canCancel && user.isGlobalAdmin === true };
  }
  async function detail(host: string, id: string, refresh = false, notify = true): Promise<RuntimeRun> {
    const key = `${host}~${id}:${refresh}:${notify}`;
    const existing = reads.get(key); if (existing) return existing;
    const version = generation, started = ++sequence;
    const task = (async () => {
      const [data, grants] = await Promise.all([request(`${route(host, id)}${refresh ? "?refresh=true" : ""}`), access(host)]);
      check(version);
      const dto = data as RuntimeDetail;
      const run = mapRuntime(dto?.activity, host, id, dto, grants);
      if (started < (sequences.get(id) ?? 0)) return runs.get(id) ?? run;
      sequences.set(id, started);
      return remember(run, notify);
    })();
    reads.set(key, task);
    try { return await task; } finally { if (reads.get(key) === task) reads.delete(key); }
  }
  function schedule() {
    if (!observed || timer) return;
    const watched = [...runs.values()].filter((run) => active(run) && !run.runtime?.unknownReason && (attempts.get(run.id) ?? 0) < 3);
    if (!watched.length) return;
    const version = generation;
    timer = setTimeout(() => {
      timer = undefined;
      void Promise.all(watched.map(async (run) => {
        attempts.set(run.id, (attempts.get(run.id) ?? 0) + 1);
        try {
          const next = await detail(run.hostId!, run.id);
          check(version);
          if (next.outcomeState === "known") attempts.set(run.id, 0);
        } catch { /* Bounded read failure retains the last observation. */ }
      })).then(() => { if (version === generation) schedule(); });
    }, 2_000);
  }
  function command(kind: "stack" | "container", target: string, command: StackCommand | ContainerCommand): Promise<string> {
    let host: string, raw: string;
    try { [host, raw] = splitInventoryId(target); } catch { return Promise.reject(runtimeError("runtime.invalid-target")); }
    const suffix = kind === "stack" ? stackRoutes[command as StackCommand] : ["start", "stop", "restart"].includes(command) ? command : undefined;
    if (!suffix) return Promise.reject(runtimeError("runtime.invalid-command"));
    const scope = `${kind}:${target}`, key = `${scope}:${command}`;
    if (ambiguous.has(scope)) return Promise.reject(runtimeError("runtime.admission-unknown"));
    const existing = pending.get(key); if (existing) return existing;
    if (ambiguous.size + pending.size + [...runs.values()].filter(active).length >= LIMIT) return Promise.reject(runtimeError("runtime.pending-limit"));
    const version = generation;
    const task = (async () => {
      const operation = kind === "stack" ? "runStackCommand" : "runContainerCommand";
      const descriptor = await capability(operation, { kind, id: target }); check(version);
      if (!descriptor.available) throw unavailable(operation, descriptor.resourceRef, descriptor.disabledReasonCode);
      let idempotencyKey: string;
      try { idempotencyKey = uuid(); } catch { throw runtimeError("runtime.key-unavailable"); }
      const began = now();
      const controller = new AbortController(); controllers.add(controller);
      const timeout = setTimeout(() => controller.abort(), COMMAND_RETRY_MS);
      try {
        for (let attempt = 0; attempt < 2; attempt++) {
          let response: Response;
          try {
            if (now() - began >= COMMAND_RETRY_MS) break;
            response = await client.fetch(`${baseUrl}/environments/${encodeURIComponent(host)}/${kind === "stack" ? "projects" : "containers"}/${encodeURIComponent(raw)}/${suffix}`, {
              method: "POST", signal: controller.signal,
              headers: { Accept: "application/json", Prefer: "respond-async", "Idempotency-Key": idempotencyKey },
            });
          } catch { check(version); if (controller.signal.aborted) break; continue; }
          check(version);
          let body: unknown;
          try { body = await response.json(); } catch { ambiguous.add(scope); throw runtimeError("runtime.admission-unknown"); }
          check(version);
          if (!response.ok) {
            const error = runtimeResponseError(body, response.status);
            if (response.status === 401) unauthorized();
            if (error.outcome.kind === "conflict" && error.outcome.activeRunId) void detail(host, error.outcome.activeRunId).catch(() => undefined);
            throw error;
          }
          const data = object(object(body).data);
          try {
            if (object(body).success !== true || (response.status === 202 ? data.code !== "run.started" : response.status !== 200 || data.code !== "run.joined") || typeof data.activityId !== "string" || !data.activityId.trim()) throw runtimeError("runtime.invalid-response");
            const run = mapRuntime(data.activity, host, data.activityId);
            run.runtime = { ...run.runtime, currentIntent: true };
            sequences.set(run.id, ++sequence);
            remember(run);
            // A final admission response ends key retention. Future work is query-only.
            void detail(host, run.id).catch(() => undefined);
            return run.id;
          } catch { ambiguous.add(scope); throw runtimeError("runtime.admission-unknown"); }
        }
        ambiguous.add(scope);
        throw runtimeError("runtime.admission-unknown");
      } finally { clearTimeout(timeout); controllers.delete(controller); }
    })();
    pending.set(key, task);
    void task.finally(() => { if (pending.get(key) === task) pending.delete(key); }).catch(() => undefined);
    return task;
  }
  async function mutate(host: string, id: string, action: "cancel" | "release", input?: RuntimeReleaseInput) {
    const version = generation;
    if (action === "release" && (input?.acknowledgeRisk !== true || !input.reason.trim() || input.reason.length > 2048)) throw runtimeError("runtime.release-invalid");
    const grants = await access(host); check(version);
    if (!(action === "cancel" ? grants.canCancel : grants.canRelease)) throw runtimeError("http.error", 403);
    const started = ++sequence;
    const data = await request(`${route(host, id)}/${action}`, { method: "POST", ...(input ? { body: JSON.stringify({ reason: input.reason.trim(), acknowledged: true }) } : {}) });
    check(version);
    const run = mapRuntime(data, host, id, undefined, grants);
    if (action === "release") {
      if (run.status !== "failed" || run.outcomeState !== "unknown") throw runtimeError("runtime.invalid-response");
      run.runtime = { ...run.runtime, released: true, releaseReason: input!.reason.trim() };
    }
    if (started >= (sequences.get(id) ?? 0)) { sequences.set(id, started); attempts.delete(id); remember(run); }
    void detail(host, id).catch(() => undefined);
    return runs.get(id) ?? run;
  }
  return {
    runStackCommand: (id: string, action: StackCommand) => command("stack", id, action),
    runContainerCommand: (id: string, action: ContainerCommand) => command("container", id, action),
    getRuntimeRun: (host: string, id: string) => { attempts.delete(id); return detail(host, id, true); },
    cancelRuntimeRun: (host: string, id: string) => mutate(host, id, "cancel"),
    releaseRuntimeRun: (host: string, id: string, input: RuntimeReleaseInput) => mutate(host, id, "release", input),
    detail,
    retained: () => [...runs.values()],
    observe(value: boolean) { observed = value; if (!value && timer) { clearTimeout(timer); timer = undefined; } if (value) schedule(); },
    reset() {
      ++generation; for (const controller of controllers) controller.abort(); controllers.clear();
      if (timer) clearTimeout(timer); timer = undefined; rights = undefined;
      runs.clear(); pending.clear(); ambiguous.clear(); reads.clear(); attempts.clear(); sequences.clear();
    },
  };
}
