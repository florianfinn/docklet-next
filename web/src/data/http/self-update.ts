// Source: docklet-next/issues/439
// Revision: 48f47c93878ecf1c27491d0214db80d5b38b547d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original self-update intake and identity-owned activity/version following.

import { CancelledError } from "@tanstack/react-query";
import type { components } from "@/api/generated/schema";
import type { CodedActionRun } from "../message-code";
import { unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import { sameProductVersion, selfUpdateRef, validSelfUpdateCheck, selfUpdateCodeKey, type SelfUpdateCheck, type SelfUpdateInput, type SelfUpdateResponse } from "@/domain/self-update";
import { SELF_UPDATE_PHASE_KEYS } from "../wire-catalogue-self-update";
import { wireKey } from "../wire-catalogue";
import { object, runtimeError } from "./runtime-mapping";
import type { CsrfClient } from "./csrf-client";
import { pausableTimeout } from "./deadline-pause";
import { maintenanceOutcome } from "./problem-code";

interface Options {
  baseUrl: string;
  client: CsrfClient;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  generation: () => number;
  readRun: (host: string, id: string, signal: AbortSignal) => Promise<CodedActionRun>;
  reload?: () => void;
}

export function createSelfUpdates({ baseUrl, client, capability, generation, readRun, reload = () => window.location.reload() }: Options) {
  let epoch = 0, reloaded = false;
  const controllers = new Set<AbortController>();
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  const following = new Set<string>();
  const pending = new Map<string, Promise<string>>();
  async function request(path: string, input?: SelfUpdateInput, parent?: AbortSignal): Promise<unknown> {
    const owner = generation(), version = epoch;
    const controller = new AbortController(); controllers.add(controller);
    const abort = () => controller.abort(); parent?.addEventListener("abort", abort, { once: true });
    if (parent?.aborted) abort();
    const stopTimer = pausableTimeout(15_000, abort);
    const current = () => { if (owner !== generation() || version !== epoch || parent?.aborted) throw new CancelledError(); };
    try {
      const response = await client.fetch(`${baseUrl}${path}`, {
        method: input ? "POST" : "GET", credentials: "same-origin", cache: "no-store", signal: controller.signal,
        headers: { Accept: "application/json", ...(input ? { "Content-Type": "application/json" } : {}) },
        // Browser claims never become the hub-owned recovery envelope.
        ...(input ? { body: JSON.stringify({ targetVersion: input.targetVersion }) } : {}),
      });
      current();
      const body: unknown = await response.json(); current();
      if (!response.ok) {
        const refused = maintenanceOutcome(response);
        if (refused) throw refused;
        const problem = object(body);
        const code = selfUpdateCodeKey(problem.code as string) ? problem.code : selfUpdateCodeKey(problem.detail as string) ? problem.detail : "http.error";
        throw runtimeError(code as string, response.status);
      }
      return body;
    } catch (error) {
      current();
      if (input && !(error instanceof Error && "outcome" in error)) throw runtimeError("update.outcome-unknown");
      throw error;
    } finally { stopTimer(); controllers.delete(controller); parent?.removeEventListener("abort", abort); }
  }
  async function allowed(host: string) {
    const owner = generation(), version = epoch;
    if (!host || host.includes("~")) throw unavailable("selfUpdate", selfUpdateRef(host));
    const cap = await capability("selfUpdate", selfUpdateRef(host));
    if (owner !== generation() || version !== epoch) throw new CancelledError();
    if (!cap.available) throw unavailable("selfUpdate", selfUpdateRef(host), cap.disabledReasonCode);
  }
  function follow(host: string, id: string, target: string) {
    if (following.has(id)) return;
    following.add(id);
    const owner = generation(), version = epoch, deadline = Date.now() + 21 * 60_000;
    let delay = 1000;
    const current = () => owner === generation() && version === epoch;
    const poll = async () => {
      timers.delete(id);
      if (!current()) return;
      const controller = new AbortController(); controllers.add(controller);
      const stopTimer = pausableTimeout(15_000, () => controller.abort());
      let done = false;
      try {
        const [run, app] = await Promise.allSettled([
          readRun(host, id, controller.signal),
          host === "0" ? request("/app-version", undefined, controller.signal) : Promise.resolve(undefined),
        ]);
        if (!current()) return;
        if (run.status === "fulfilled" && run.value.id === id && run.value.hostId === host) {
          const status = run.value.status;
          const active = status === "running" || status === "queued";
          done = !active && status !== "succeeded" || host !== "0" && !active;
          const info = (app.status === "fulfilled" ? app.value : undefined) as components["schemas"]["VersionInfo"] | undefined;
          if (host === "0" && status === "succeeded" && sameProductVersion(info?.currentVersion, target)) {
            done = true;
            if (!reloaded) { reloaded = true; reload(); }
          }
        }
      } finally {
        stopTimer(); controllers.delete(controller);
        if (current() && !done && Date.now() < deadline) {
          timers.set(id, setTimeout(() => { void poll(); }, delay));
          delay = Math.min(delay * 2, 10_000);
        } else if (current()) following.delete(id);
      }
    };
    timers.set(id, setTimeout(() => { void poll(); }, delay));
  }
  return {
    async getSelfUpdate(host: string): Promise<SelfUpdateCheck> {
      await allowed(host);
      const value = await request(`/environments/${encodeURIComponent(host)}/system/upgrade/check`) as SelfUpdateCheck;
      if (!validSelfUpdateCheck(value)) throw runtimeError("update.check-unavailable");
      // Recover following after navigation or a browser refresh from the durable operation.
      if (value.operation?.runId && value.operation.phase !== "settled") follow(host, value.operation.runId, value.operation.targetVersion);
      return value;
    },
    selfUpdate(host: string, input: SelfUpdateInput): Promise<string> {
      const existing = pending.get(host); if (existing) return existing;
      const task = (async () => {
        await allowed(host);
        if (!input.targetVersion?.trim()) throw runtimeError("update.invalid-target");
        const row = await request(`/environments/${encodeURIComponent(host)}/system/upgrade`, input) as SelfUpdateResponse;
        if (row?.accepted !== true) throw runtimeError(selfUpdateCodeKey(row?.code) ? row.code! : "update.check-unavailable");
        if (!row.opId || !row.runId || !wireKey(SELF_UPDATE_PHASE_KEYS, row.phase)) throw runtimeError("update.check-unavailable");
        follow(host, row.runId, input.targetVersion);
        return row.runId;
      })();
      pending.set(host, task);
      void task.finally(() => { if (pending.get(host) === task) pending.delete(host); }).catch(() => {});
      return task;
    },
    reset() {
      ++epoch; timers.forEach(clearTimeout); timers.clear(); controllers.forEach(c => c.abort()); controllers.clear(); following.clear(); pending.clear();
    },
  };
}
