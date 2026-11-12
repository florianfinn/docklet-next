// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original phase 2a stack update consumer for issue #231.

import { CancelledError, isCancelledError } from "@tanstack/react-query";
import type { UpdatesApi, PolicyMode } from "@/domain/stack-updates";
import type { CsrfClient } from "./csrf-client";
import { unavailable, AdapterOutcomeError, type CapabilityDescriptor, type ResourceRef } from "../capability";
import { splitInventoryId } from "./inventory-mapping";
import { record, list, choice, reason, id as requiredId, invalidUpdate } from "./update-validation";
import { baseline, points, policy, hints, checkResult } from "./update-decoders";
import { runtimeResponseError } from "./runtime-mapping";
import { MAINTENANCE_CODE } from "./problem-code";
interface Options { baseUrl: string; client: CsrfClient; capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>; generation: () => number; changed: () => void }
export function createStackUpdates({ baseUrl, client, capability, generation, changed }: Options): Required<Omit<UpdatesApi, "checkAllUpdates" | "restoreContainerImage" | "getContainerPreviousImage" | "getUpdateSettings">> {
  async function request(operation: string, ref: ResourceRef, suffix: string, method = "GET", body?: object, reads: string[] = []): Promise<unknown> {
    const owner = generation(), current = () => { if (owner !== generation()) throw new CancelledError(); };
    for (const name of [operation, ...reads]) { const cap = await capability(name, ref); current(); if (!cap.available) throw unavailable(name, cap.resourceRef, cap.disabledReasonCode); }
    let response: Response;
    const host = ref.kind === "stack" || ref.kind === "container" ? splitInventoryId(ref.id)[0] : ref.id;
    try { response = await client.fetch(`${baseUrl}/environments/${encodeURIComponent(host)}/updater/${suffix}`, { method, credentials: "same-origin", headers: { Accept: "application/json", ...(body ? { "Content-Type": "application/json" } : {}) }, ...(body ? { body: JSON.stringify(body) } : {}) }); }
    catch (error) { current(); if (error instanceof AdapterOutcomeError) throw error; throw new AdapterOutcomeError({ kind: "error", code: method === "GET" ? "http.error" : "runtime.admission-unknown" }); }
    current();
    let raw: unknown; try { raw = await response.json(); } catch { current(); throw new AdapterOutcomeError({ kind: "error", code: method === "GET" ? "http.error" : "runtime.admission-unknown" }); }
    current();
    if (!response.ok) {
      const error = runtimeResponseError(raw, response.status);
      if (method !== "GET" && (response.status === 502 || response.status === 504) && error.outcome.kind === "error" && error.outcome.code === "agent.unavailable") throw new AdapterOutcomeError({ kind: "error", code: "runtime.admission-unknown" });
      if (error.outcome.kind === "error") throw new AdapterOutcomeError({ ...error.outcome, code: ["agent.unavailable", "agent.invalid-token", "agent.update-required", MAINTENANCE_CODE].includes(error.outcome.code) ? error.outcome.code : reason(error.outcome.code) ?? "http.error" });
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
    async deleteRestorePoint(id, pointId) { const s = stack(id); requiredId(pointId); return save(async () => { const data = record(await request("deleteRestorePoint", s.ref, `project/${s.path}/restore-points/${encodeURIComponent(pointId)}`, "DELETE", undefined, ["listRestorePoints"])); requiredId(data.message); }); },
    async deleteHostRestorePoint(host, pointId) { requiredId(pointId); return save(async () => { const data = record(await request("deleteHostRestorePoint", { kind: "host", id: host }, `restore-points/${encodeURIComponent(pointId)}`, "DELETE", undefined, ["listHostRestorePoints"])); requiredId(data.message); }); },
    async getAutoUpdatePolicy(id) { const s = stack(id); return policy(await request("getAutoUpdatePolicy", s.ref, `policies/stack/${s.path}`), s.host, s.raw); },
    async saveAutoUpdatePolicy(id, mode: PolicyMode, expectedRevision, window) { const s = stack(id); return save(async () => policy(await request(mode === "scheduled" ? "setAutoUpdateApply" : "setAutoUpdate", s.ref, `policies/stack/${s.path}`, "PUT", { mode, expectedRevision, ...(mode === "scheduled" ? { window } : {}) }, ["getAutoUpdatePolicy"]), s.host, s.raw)); },
    async listUpdatePolicies(host) { return list(await request("listUpdatePolicies", { kind: "host", id: host }, "policies")).map((v) => { const o = record(v); return policy(v, host, requiredId(o.targetId), choice(o.targetKind, ["container", "stack"])); }); },
    async getContainerUpdatePolicy(id) { const s = stack(id); return policy(await request("getAutoUpdatePolicy", { kind: "container", id }, `policies/container/${s.path}`), s.host, s.raw, "container"); },
    async saveContainerUpdatePolicy(id, mode, expectedRevision, window) { const s = stack(id); return save(async () => policy(await request(mode === "scheduled" ? "setAutoUpdateApply" : "setAutoUpdate", { kind: "container", id }, `policies/container/${s.path}`, "PUT", { mode, expectedRevision, ...(mode === "scheduled" ? { window } : {}) }, ["getAutoUpdatePolicy"]), s.host, s.raw, "container")); },
    async getSystemImageHints(host) { return hints(await request("listSystemComponents", { kind: "host", id: host }, "system-image-hints")); },
    async checkStackUpdates(id) { const s = stack(id); try { checkResult(await request("checkUpdates", s.ref, `projects/${s.path}/check`, "POST")); } finally { changed(); } },
  };
}
