// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original phase 2a stack update consumer for issue #231.

import { CancelledError, isCancelledError } from "@tanstack/react-query";
import type { UpdatesApi, PolicyMode, ScheduleInput } from "@/domain/stack-updates";
import type { CsrfClient } from "./csrf-client";
import { unavailable, AdapterOutcomeError, type CapabilityDescriptor, type ResourceRef } from "../capability";
import { splitInventoryId } from "./inventory-mapping";
import { record, reason, invalidUpdate } from "./update-validation";
import { baseline, points, policy, schedules, hints, checkResult } from "./update-decoders";
import { runtimeResponseError } from "./runtime-mapping";
interface Options { baseUrl: string; client: CsrfClient; capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>; generation: () => number; changed: () => void }
export function createStackUpdates({ baseUrl, client, capability, generation, changed }: Options): Required<UpdatesApi> {
  async function request(operation: string, ref: ResourceRef, suffix: string, method = "GET", body?: object, reads: string[] = []): Promise<unknown> {
    const owner = generation(), current = () => { if (owner !== generation()) throw new CancelledError(); };
    for (const name of [operation, ...reads]) { const cap = await capability(name, ref); current(); if (!cap.available) throw unavailable(name, cap.resourceRef, cap.disabledReasonCode); }
    let response: Response;
    const host = ref.kind === "stack" ? splitInventoryId(ref.id)[0] : ref.id;
    try { response = await client.fetch(`${baseUrl}/environments/${encodeURIComponent(host)}/updater/${suffix}`, { method, credentials: "same-origin", headers: { Accept: "application/json", ...(body ? { "Content-Type": "application/json" } : {}) }, ...(body ? { body: JSON.stringify(body) } : {}) }); }
    catch (error) { current(); if (error instanceof AdapterOutcomeError) throw error; throw new AdapterOutcomeError({ kind: "error", code: method === "GET" ? "http.error" : "runtime.admission-unknown" }); }
    current();
    let raw: unknown; try { raw = await response.json(); } catch { current(); throw new AdapterOutcomeError({ kind: "error", code: method === "GET" ? "http.error" : "runtime.admission-unknown" }); }
    current();
    if (!response.ok) {
      const error = runtimeResponseError(raw, response.status);
      if (method !== "GET" && (response.status === 502 || response.status === 504) && error.outcome.kind === "error" && error.outcome.code === "agent.unavailable") throw new AdapterOutcomeError({ kind: "error", code: "runtime.admission-unknown" });
      if (error.outcome.kind === "error") throw new AdapterOutcomeError({ ...error.outcome, code: reason(error.outcome.code) ?? "http.error" });
      throw error;
    }
    let envelope: Record<string, unknown>;
    try { envelope = record(raw); if (envelope.success !== true || !("data" in envelope)) throw invalidUpdate(); } catch (error) { if (method === "GET") throw error; throw new AdapterOutcomeError({ kind: "error", code: "runtime.admission-unknown" }); }
    return envelope.data;
  }
  async function save<T>(action: () => Promise<T>): Promise<T> {
    try { return await action(); }
    catch (error) { if (!isCancelledError(error) && error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.code === "http.error" && error.outcome.status === undefined) throw new AdapterOutcomeError({ kind: "error", code: "runtime.admission-unknown" }); throw error; }
    finally { changed(); }
  }
  const stack = (id: string) => { const [host, raw] = splitInventoryId(id); return { host, raw, ref: { kind: "stack" as const, id }, path: encodeURIComponent(raw) }; };
  return {
    async getUpdateBaseline(id) { const s = stack(id); return baseline(await request("getUpdateBaseline", s.ref, `projects/${s.path}/baseline`)); },
    async listHostRestorePoints(host) { return points(await request("listHostRestorePoints", { kind: "host", id: host }, "restore-points")); },
    async listRestorePoints(id) { const s = stack(id); return points(await request("listRestorePoints", s.ref, `project/${s.path}/restore-points`), s.raw); },
    async getAutoUpdatePolicy(id) { const s = stack(id); return policy(await request("getAutoUpdatePolicy", s.ref, `policies/project/${s.path}`), s.host, s.raw); },
    async saveAutoUpdatePolicy(id, mode: PolicyMode, expectedRevision) { const s = stack(id); return save(async () => policy(await request(mode === "apply" ? "setAutoUpdateApply" : "setAutoUpdate", s.ref, `policies/project/${s.path}`, "PUT", { mode, expectedRevision }, ["getAutoUpdatePolicy"]), s.host, s.raw)); },
    async getUpdateSchedules(host) { return schedules(await request("listSchedule", { kind: "host", id: host }, "schedules"), host); },
    async saveUpdateSchedule(host, input: ScheduleInput) { return save(async () => schedules(await request("saveUpdateSchedule", { kind: "host", id: host }, "schedules", "PUT", input, ["listSchedule"]), host)); },
    async getSystemImageHints(host) { return hints(await request("listSystemComponents", { kind: "host", id: host }, "system-image-hints")); },
    async checkStackUpdates(id) { const s = stack(id); try { checkResult(await request("checkUpdates", s.ref, `projects/${s.path}/check`, "POST")); } finally { changed(); } },
  };
}
