// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original v14 update settings and previous-image reads. M24 S6: observed recovery and update UI follow-ups.

import { CancelledError } from "@tanstack/react-query";
import type { UpdatesApi } from "@/domain/stack-updates";
import { AdapterOutcomeError, unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import type { CsrfClient } from "./csrf-client";
import { splitInventoryId } from "./inventory-mapping";
import { runtimeResponseError } from "./runtime-mapping";
import { record, id, date, choice, bool, optionalDate, list, reason } from "./update-validation";

export function createUpdateSettings(o: { baseUrl: string; client: CsrfClient; generation: () => number; capability: (op: string, ref: ResourceRef) => Promise<CapabilityDescriptor> }): Required<Pick<UpdatesApi, "getUpdateSettings" | "checkAllUpdates" | "getContainerPreviousImage" | "getUpdateCheckJob" | "getUpdateJobTargets">> {
  async function request(path: string, method = "GET", operation?: string, ref: ResourceRef = { kind: "hub", id: "0" }) {
    const version = o.generation(), current = () => { if (version !== o.generation()) throw new CancelledError(); };
    if (operation) { const cap = await o.capability(operation, ref); current(); if (!cap.available) throw unavailable(operation, ref, cap.disabledReasonCode); }
    let response: Response;
    try { response = await o.client.fetch(`${o.baseUrl}${path}`, { method, headers: { Accept: "application/json" } }); } catch { current(); throw new AdapterOutcomeError({ kind: "error", code: method === "GET" ? "http.error" : "runtime.admission-unknown" }); }
    current();
    let raw: unknown;
    try { raw = await response.json(); } catch { current(); throw new AdapterOutcomeError({ kind: "error", code: method === "GET" ? "http.error" : "runtime.admission-unknown" }); }
    current();
    if (!response.ok) throw runtimeResponseError(raw, response.status);
    return raw;
  }
  async function checkJob(host: string) {
    const jobs = record(await request(`/environments/${encodeURIComponent(host)}/jobs`));
    const job = list(jobs.jobs).map(record).find((j) => j.id === "update-check");
    if (!job) throw new AdapterOutcomeError({ kind: "error", code: "http.error" });
    return { enabled: bool(job.enabled), schedule: id(job.schedule), nextRun: optionalDate(job.nextRun ?? undefined) };
  }
  return {
    getUpdateCheckJob: checkJob,
    async getUpdateJobTargets(host, jobId, runId) {
      const run = record(await request(`/environments/${encodeURIComponent(host)}/jobs/${encodeURIComponent(jobId)}/runs/${encodeURIComponent(runId)}`));
      if (id(run.id) !== runId || id(run.jobId) !== jobId || id(run.environmentId) !== host) throw new AdapterOutcomeError({ kind: "error", code: "http.error" });
      return list(record(run.outcome).targets ?? null).map((raw) => { const t = record(raw); return { id: id(t.id), status: choice(t.status, ["queued", "waiting", "running", "retrying", "succeeded", "partial", "skipped", "failed", "needs_attention", "canceled"]), code: reason(t.code), activityId: t.activityId === undefined ? undefined : id(t.activityId), environmentId: t.environmentId === undefined ? undefined : id(t.environmentId), targetId: t.targetId === undefined ? undefined : id(t.targetId) }; });
    },
    async getUpdateSettings() {
      const raw = await request("/environments/0/settings", "GET", "saveSettings");
      if (!Array.isArray(raw)) throw new AdapterOutcomeError({ kind: "error", code: "http.error" });
      const values = Object.fromEntries(raw.map((v) => { const r = record(v); return [id(r.key), typeof r.value === "string" ? r.value : id(r.value)]; }));
      const timeZone = id(values.effectiveTimeZone); new Intl.DateTimeFormat("en", { timeZone });
      const interval = choice(values.updateCheckInterval, ["6h", "12h", "24h"]), notify = choice(values.updateNotify, ["true", "false"]) === "true";
      let nextRun: string | undefined, projectionUnavailable = false;
      try {
        nextRun = (await checkJob("0")).nextRun;
      } catch (error) { if (error instanceof CancelledError) throw error; projectionUnavailable = true; }
      return { interval, notify, timeZone, nextRun, projectionUnavailable };
    },
    async checkAllUpdates() {
      const envelope = record(await request("/updater/check-all", "POST", "checkAllUpdates"));
      if (envelope.success !== true) throw new AdapterOutcomeError({ kind: "error", code: "runtime.admission-unknown" });
      return id(envelope.runId);
    },
    async getContainerPreviousImage(containerId) {
      const [host, raw] = splitInventoryId(containerId);
      const envelope = record(await request(`/environments/${encodeURIComponent(host)}/containers/${encodeURIComponent(raw)}`, "GET", "recreateContainer", { kind: "container", id: containerId }));
      if (envelope.success !== true) throw new AdapterOutcomeError({ kind: "error", code: "http.error" });
      const details = record(envelope.data);
      if (details.id !== raw) throw new AdapterOutcomeError({ kind: "error", code: "update.target-changed" });
      if (details.previousImage === undefined) return undefined;
      const image = record(details.previousImage);
      return { imageId: id(image.imageId), reference: id(image.imageRef), createdAt: date(image.createdAt) };
    },
  };
}
