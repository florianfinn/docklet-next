// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { CancelledError, isCancelledError } from "@tanstack/react-query";
import type { ContainerCommand, RuntimeReleaseInput, StackCommand } from "../adapter";
import { AdapterOutcomeError, unavailable } from "../capability";
import type { CapabilityDescriptor, ResourceRef } from "../capability";
import type { CodedActionRun } from "../message-code";
import type { CsrfClient } from "./csrf-client";
import { splitInventoryId } from "./inventory-mapping";
import { isRuntimeTokenReadError } from "./runtime-token-read";
import { mapRuntime, object, runtimeError, runtimeResponseError, type RuntimeDetail, type RuntimeRun } from "./runtime-mapping";

export const COMMAND_RETRY_MS = 10 * 60_000;
export const RIGHTS_MAX_AGE_MS = 30_000;
const LIMIT = 256;
const stackRoutes: Record<StackCommand, string> = { start: "up", stop: "down", restart: "restart", redeploy: "redeploy" };
const active = (run: Pick<CodedActionRun, "status">) => run.status === "running" || run.status === "queued";

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
  publish: (run: RuntimeRun, detailed?: boolean) => void;
  unauthorized?: () => void;
  now?: () => number;
  uuid?: () => string;
}

/** Commands own only browser intent and observation. The hub owns admission and effects. */
export function createRuntimeCommands({ baseUrl, client, capability, publish, unauthorized = () => {}, now = () => performance.now(), uuid = commandUUID }: Options) {
  let generation = 0;
  let observed = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let rights: { task: Promise<Record<string, unknown>>; readAt?: number } | undefined;
  const controllers = new Set<AbortController>();
  const pending = new Map<string, Promise<string>>();
  const ambiguous = new Set<string>();
  const runs = new Map<string, RuntimeRun>();
  const attempts = new Map<string, number>();
  const reads = new Map<string, Promise<RuntimeRun>>();
  const signalKeys = new WeakMap<AbortSignal, number>();
  const detailOwners = new Map<string, number>();
  const explicitFollows = new Set<string>();
  let signalKey = 0;
  const sequences = new Map<string, number>();
  let sequence = 0;
  const check = (version: number) => { if (version !== generation) throw new CancelledError(); };
  const route = (host: string, id: string) => `${baseUrl}/environments/${encodeURIComponent(host)}/runtime-runs/${encodeURIComponent(id)}`;

  function remember(run: RuntimeRun, notify = true, detailed = false) {
    const prior = runs.get(run.id);
    if (prior && prior.hostId !== run.hostId) throw runtimeError("runtime.invalid-response");
    run.runtime = { ...run.runtime, currentIntent: prior?.runtime?.currentIntent || run.runtime?.currentIntent,
      sourceObserved: prior?.runtime?.sourceObserved || run.runtime?.sourceObserved };
    if (!runs.has(run.id) && runs.size >= LIMIT) {
      const finished = [...runs.values()].find((row) => !active(row));
      if (finished) { runs.delete(finished.id); attempts.delete(finished.id); sequences.delete(finished.id); }
      else { sequences.delete(run.id); attempts.delete(run.id); if (notify) publish(run, detailed); return run; }
    }
    runs.set(run.id, run);
    if (notify) publish(run, detailed);
    schedule();
    return run;
  }
  async function request(url: string, init: RequestInit = {}): Promise<unknown> {
    const version = generation;
    const controller = new AbortController(); controllers.add(controller);
    const abort = () => controller.abort();
    init.signal?.addEventListener("abort", abort, { once: true });
    if (init.signal?.aborted) abort();
    const timeout = setTimeout(() => controller.abort(), url.includes("/runtime-runs/") && !init.method ? 40_000 : 15_000);
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
      check(version);
      if (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status === 401) { unauthorized(); throw error; }
      throw error instanceof AdapterOutcomeError ? error : runtimeError("http.error");
    }
    finally { clearTimeout(timeout); init.signal?.removeEventListener("abort", abort); controllers.delete(controller); }
  }
  async function access(host: string) {
    const version = generation;
    if (!rights || (rights.readAt !== undefined && now() - rights.readAt >= RIGHTS_MAX_AGE_MS)) {
      const entry: NonNullable<typeof rights> = { task: request(`${baseUrl}/auth/me`).then((data) => {
        check(version);
        const user = object(data), permissions = object(user.permissionsByEnv);
        // The hub serializes an empty environment permission slice as null.
        if (typeof user.isGlobalAdmin !== "boolean" || !user.permissionsByEnv || typeof user.permissionsByEnv !== "object" || Array.isArray(user.permissionsByEnv) ||
            Object.values(permissions).some((list) => list !== null && (!Array.isArray(list) || list.some((item) => typeof item !== "string")))) {
          throw runtimeError("runtime.permissions-unavailable");
        }
        if (rights === entry) entry.readAt = now();
        return user;
      }) };
      rights = entry;
      void entry.task.catch(() => { if (rights === entry) rights = undefined; });
    }
    let user: Record<string, unknown>;
    try { user = await rights.task; }
    catch (error) {
      check(version);
      if (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status === 401) throw error;
      throw runtimeError("runtime.permissions-unavailable");
    }
    check(version);
    const permissions = object(user.permissionsByEnv);
    const allowed = (permission: string) => [permissions.global, permissions[host]].some((list) => Array.isArray(list) && list.includes(permission));
    const canCancel = allowed("activities:read") && allowed("activities:cancel");
    return { canCancel, canRelease: canCancel && user.isGlobalAdmin === true };
  }
  async function detail(host: string, id: string, refresh = false, notify = true, signal?: AbortSignal): Promise<RuntimeRun> {
    if (signal && !signalKeys.has(signal)) signalKeys.set(signal, ++signalKey);
    const key = `${host}~${id}:${refresh}:${notify}:${signal ? signalKeys.get(signal) : "shared"}`;
    const existing = reads.get(key); if (existing) return existing;
    const version = generation, started = ++sequence;
    const task = (async () => {
      const [data, grants] = await Promise.all([request(`${route(host, id)}${refresh ? "?refresh=true" : ""}`, { signal }), access(host).catch((error: unknown) => {
        if (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.code === "runtime.permissions-unavailable") return { permissionsUnavailable: true };
        throw error;
      })]);
      check(version);
      if (signal?.aborted) throw new CancelledError();
      const dto = data as RuntimeDetail;
      const run = mapRuntime(dto?.activity, host, id, dto, grants);
      if (runs.has(id) && runs.get(id)!.hostId !== host) throw runtimeError("runtime.invalid-response");
      if (started < (sequences.get(id) ?? 0)) return runs.get(id) ?? run;
      sequences.set(id, started);
      return remember(run, notify, true);
    })();
    reads.set(key, task);
    try { return await task; } finally { if (reads.get(key) === task) reads.delete(key); }
  }
  async function project(run: CodedActionRun): Promise<CodedActionRun> {
    if (run.outcomeState === undefined) return run;
    const version = generation, started = ++sequence;
    try { return await detail(run.hostId!, run.id, false, false); }
    catch (error) {
      check(version);
      if (isCancelledError(error) || (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status === 401)) throw error;
      const retained = runs.get(run.id);
      if (!retained || retained.hostId !== run.hostId) return run;
      // A settled generic row can supersede only active retained recovery after a failed read.
      if (active(run) || !active(retained) || (sequences.get(run.id) ?? 0) > started ||
          (retained.status === run.status && retained.outcomeState === run.outcomeState)) return retained;
      sequences.set(run.id, started);
      return remember({ ...run, runtime: undefined } as RuntimeRun, false);
    }
  }
  function mergeSummary(run: CodedActionRun): CodedActionRun {
    const prior = runs.get(run.id);
    if (!prior || prior.hostId !== run.hostId) return run;
    if ((!active(prior) && active(run)) || (prior.outcomeState === "known" && run.outcomeState === "unknown")) return prior;
    const incoming = run as RuntimeRun;
    const same = prior.status === run.status && prior.outcomeState === run.outcomeState;
    const merged = { ...prior, ...incoming, cancelRequested: run.cancelRequested || prior.cancelRequested,
      runtime: { ...(same ? prior.runtime : {}), ...incoming.runtime, currentIntent: prior.runtime?.currentIntent } } as RuntimeRun;
    if (same && !incoming.runtime?.unknownReason && prior.runtime?.unknownReason) merged.runtime!.unknownReason = prior.runtime.unknownReason;
    runs.set(run.id, merged);
    return merged;
  }
  function schedule() {
    if (!observed || timer) return;
    const watched = [...runs.values()].filter((run) => active(run) && (run.runtime?.currentIntent || detailOwners.has(run.id) || explicitFollows.has(run.id)) && !run.runtime?.unknownReason && (attempts.get(run.id) ?? 0) < 3);
    if (!watched.length) return;
    const version = generation;
    timer = setTimeout(() => {
      timer = undefined;
      void Promise.all(watched.filter((run) => run.runtime?.currentIntent || detailOwners.has(run.id) || explicitFollows.has(run.id)).map(async (run) => {
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
      let uncertainSend = false;
      const controller = new AbortController(); controllers.add(controller);
      const timeout = setTimeout(() => controller.abort(), COMMAND_RETRY_MS);
      try {
        // Token bootstrap can throw a raw transport error before any command is sent.
        try { await client.token(); }
        catch (error) {
          check(version);
          if (error instanceof AdapterOutcomeError) {
            if (error.outcome.kind === "error" && error.outcome.status === 401) unauthorized();
            throw error;
          }
          throw runtimeError("runtime.pre-admission");
        }
        check(version);
        if (controller.signal.aborted || now() - began >= COMMAND_RETRY_MS) throw runtimeError("runtime.pre-admission");
        for (let attempt = 0; attempt < 2; attempt++) {
          const started = ++sequence;
          let response: Response;
          try {
            if (now() - began >= COMMAND_RETRY_MS) break;
            response = await client.fetch(`${baseUrl}/environments/${encodeURIComponent(host)}/${kind === "stack" ? "projects" : "containers"}/${encodeURIComponent(raw)}/${suffix}`, {
              method: "POST", signal: controller.signal,
              headers: { Accept: "application/json", Prefer: "respond-async", "Idempotency-Key": idempotencyKey },
            });
          } catch (error) {
            check(version);
            // CSRF bootstrap errors prove this attempt never passed request protection.
            if (error instanceof AdapterOutcomeError || isRuntimeTokenReadError(error)) {
              if (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status === 401) { unauthorized(); throw error; }
              if (uncertainSend) break;
              throw error instanceof AdapterOutcomeError ? error : runtimeError("runtime.pre-admission");
            }
            uncertainSend = true;
            if (controller.signal.aborted) break;
            continue;
          }
          check(version);
          let body: unknown;
          try { body = await response.json(); } catch { check(version); ambiguous.add(scope); throw runtimeError("runtime.admission-unknown"); }
          check(version);
          if (!response.ok) {
            const error = runtimeResponseError(body, response.status);
            if (response.status === 401) { unauthorized(); throw error; }
            if (uncertainSend) { ambiguous.add(scope); throw runtimeError("runtime.admission-unknown"); }
            if (error.outcome.kind === "conflict" && error.outcome.activeRunId) void detail(host, error.outcome.activeRunId).catch(() => undefined);
            throw error;
          }
          const data = object(object(body).data);
          try {
            if (object(body).success !== true || (response.status === 202 ? data.code !== "run.started" : response.status !== 200 || data.code !== "run.joined") || typeof data.activityId !== "string" || !data.activityId.trim()) throw runtimeError("runtime.invalid-response");
            let run = mapRuntime(data.activity, host, data.activityId);
            const retained = runs.get(run.id);
            // The admission snapshot can arrive after a newer targeted observation.
            if (retained?.hostId === host && started < (sequences.get(run.id) ?? 0)) run = retained;
            run.runtime = { ...run.runtime, currentIntent: true };
            sequences.set(run.id, Math.max(started, sequences.get(run.id) ?? 0));
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
    getRuntimeRun: (host: string, id: string) => { explicitFollows.add(id); attempts.delete(id); return detail(host, id, true); },
    cancelRuntimeRun: (host: string, id: string) => mutate(host, id, "cancel"),
    releaseRuntimeRun: (host: string, id: string, input: RuntimeReleaseInput) => mutate(host, id, "release", input),
    detail, project, mergeSummary,
    observeDetail(id: string, value: boolean) {
      const count = (detailOwners.get(id) ?? 0) + (value ? 1 : -1);
      if (count > 0) detailOwners.set(id, count); else detailOwners.delete(id);
      if (!value && ![...runs.values()].some((run) => active(run) && (run.runtime?.currentIntent || detailOwners.has(run.id) || explicitFollows.has(run.id)))) {
        if (timer) clearTimeout(timer); timer = undefined;
      }
      schedule();
    },
    retained: () => [...runs.values()],
    observe(value: boolean) { observed = value; if (!value && timer) { clearTimeout(timer); timer = undefined; } if (value) schedule(); },
    reset(retainOwners = false) {
      ++generation; for (const controller of controllers) controller.abort(); controllers.clear();
      if (timer) clearTimeout(timer); timer = undefined; rights = undefined;
      runs.clear(); pending.clear(); ambiguous.clear(); reads.clear(); attempts.clear(); sequences.clear(); if (!retainOwners) detailOwners.clear(); explicitFollows.clear();
    },
  };
}
