// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: cf5aa27a8ca449d95c66dcd253f7a1a2e114f0f5
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original live vulnerability UI and contract regressions for issue #262.

import { isCancelledError } from "@tanstack/react-query";
import type { DockletAdapter } from "../adapter";
import { AdapterOutcomeError, unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import type { DockerImage } from "@/domain/images";
import { imageReference } from "@/domain/images";
import type { Finding, Host, ScannedImage, SecurityOverview } from "@/domain/model";
import { ScanAdmissionError, type ScanAdmission } from "@/domain/security";
import { splitInventoryId } from "./inventory-mapping";
import { array, count, counts, finding, id, invalid, object, run, summaries } from "./vulnerability-decoders";
import { createSecuritySettings, settingsValues } from "./security-settings";
import { vulnerabilityTransport, VulnerabilityFailure } from "./vulnerability-transport";
import type { CsrfClient } from "./csrf-client";

interface Options {
  baseUrl: string; client: CsrfClient; generation: () => number;
  capability: (op: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  hosts: () => Promise<Host[]>; images: () => Promise<DockerImage[]>;
  changed: () => void; accepted: (host: string, activity: string) => void;
}
const empty = () => ({ critical: 0, high: 0, medium: 0, low: 0, unknown: 0 });
function errorCode(error: unknown): string {
  if (error instanceof VulnerabilityFailure) return error.code;
  if (error instanceof AdapterOutcomeError) {
    if (error.outcome.kind === "not-available") return error.outcome.reasonCode;
    if ("code" in error.outcome) return error.outcome.code;
  }
  return "vulnerability.read-unavailable";
}

/** Durable run counters are the scan progress producer; activity progress is not populated. */
export async function readScanProgress(request: ReturnType<typeof vulnerabilityTransport>, host: string, activity: string, signal?: AbortSignal) {
  const r = run(object(await request(`/environments/${encodeURIComponent(host)}/vulnerabilities/runs/${encodeURIComponent(activity)}`, "GET", undefined, true, signal)).data);
  if (r.activityId !== activity || r.environmentId !== "0") invalid();
  return r.targets ? Math.min(100, Math.round(100 * (r.scanned + r.failed + r.skipped) / r.targets)) : undefined;
}
export function createVulnerabilities(o: Options): Pick<DockletAdapter, "getSecurity" | "runScan" | "setScanEnabled" | "getScanSettings" | "saveScanSettings" | "cancelScan"> {
  const request = vulnerabilityTransport(o.baseUrl, o.client, o.generation);
  const settings = createSecuritySettings(request, o.capability, o.changed);
  async function require(operation: string, ref: ResourceRef) { const c = await o.capability(operation, ref); if (!c.available) throw unavailable(operation, ref, c.disabledReasonCode); }
  async function hostImages(host: Host, images: DockerImage[]) {
    await require("getSecurity", { kind: "host", id: host.id });
    const local = images.filter((i) => i.hostId === host.id);
    const ids = local.map((i) => splitInventoryId(i.id)[1]);
    const base = `/environments/${encodeURIComponent(host.id)}`;
    const [batch, history] = await Promise.all([
      request(`${base}/images/vulnerabilities/summaries`, "POST", { imageIds: ids }),
      request(`${base}/vulnerabilities/runs`),
    ]);
    const summary = summaries(object(batch).data);
    const rows: ScannedImage[] = local.map((im) => {
      const s = summary.get(splitInventoryId(im.id)[1]);
      const usage = im.consumers.some((c) => c.state === "running") ? "running" : im.consumers.length ? "stopped" : "unused";
      return { id: im.id, ref: imageReference(im), hostId: host.id, stackId: im.consumers.find((c) => c.stackId)?.stackId, usage,
        status: s?.status ?? "pending", unavailable: !!im.inventory?.stale,
        code: s?.errorCode, platform: s?.scannedPlatform, counts: s?.summary ? counts(s.summary) : empty(), scannedAt: s?.completedAt ?? "" };
    });
    // One capped response avoids page cuts across changing exposure/intelligence scores.
    const findings: Finding[] = [];
    const localIds = new Set(ids);
    const page = object(await request(`${base}/vulnerabilities/all?limit=-1&sort=vulnerabilityId&order=asc`));
    const data = array(page.data), total = count(object(page.pagination).totalItems);
    if (data.length !== total) invalid();
    for (const raw of data) {
      const f = finding(raw, host.id);
      if (object(raw).ignored === true || !localIds.has(f.imageId!)) continue;
      findings.push(f);
    }
    // The full active finding list excludes ignores; raw batch summaries retain scan-time totals.
    for (const row of rows) if (!row.unavailable && row.scannedAt) {
      const active = empty();
      for (const f of findings) if (f.imageId === splitInventoryId(row.id!)[1]) active[f.severity]++;
      row.counts = active;
    }
    const historyRows = array(object(history).data, 30).map((raw) => {
      const r = run(raw);
      if (r.environmentId !== "0" || r.status === "running") invalid();
      return { id: r.activityId, hostId: host.id, at: r.startedAt, trigger: r.origin, status: r.status as Exclude<typeof r.status, "running">,
        sampled: r.status === "completed" || r.status === "partial" && r.scanned > 0, counts: counts(r) };
    });
    return { images: rows, findings, history: historyRows };
  }
  return {
    ...settings,
    async getSecurity() {
      await require("getSecurity", { kind: "hub", id: "0" });
      const values = settingsValues(await request("/environments/0/settings/public", "GET", undefined, false));
      if (!["true", "false"].includes(values.featureVulnerabilityManagementEnabled ?? "")) invalid();
      const result: SecurityOverview = { enabled: values.featureVulnerabilityManagementEnabled === "true", schedule: { mode: "manual" }, images: [], findings: [], history: [], unavailable: [] };
      if (!result.enabled) return result;
      const images = await o.images();
      const hosts = await o.hosts();
      // Aggregate complete batches once: a fast host cannot erase a slower host's cached rows.
      const batches = await Promise.all(hosts.map(async (host) => {
        try {
          const batch = await hostImages(host, images);
          if (host.inventory?.stale || batch.images.some((i) => i.unavailable)) result.unavailable!.push({ hostId: host.id, code: "vulnerability.read-unavailable" });
          return batch;
        }
        catch (error) {
          if (isCancelledError(error)) throw error;
          result.unavailable!.push({ hostId: host.id, code: errorCode(error) });
          return { images: images.filter((i) => i.hostId === host.id).map((i): ScannedImage => ({ id: i.id, ref: imageReference(i), hostId: host.id, usage: i.consumers.some((c) => c.state === "running") ? "running" : i.consumers.length ? "stopped" : "unused", counts: empty(), scannedAt: "", unavailable: true })), findings: [], history: [] };
        }
      }));
      result.images = batches.flatMap((b) => b.images); result.findings = batches.flatMap((b) => b.findings); result.history = batches.flatMap((b) => b.history);
      return result;
    },
    async runScan(input) {
      let selected: string[];
      let image: string | undefined;
      if (input?.imageId) { const [host, raw] = splitInventoryId(input.imageId); selected = [host]; image = raw; }
      else selected = [...(input?.hostIds?.length ? input.hostIds : (await o.hosts()).map((h) => h.id))];
      const admissions: ScanAdmission[] = await Promise.all(selected.map(async (hostId) => {
        const ref: ResourceRef = image ? { kind: "image", id: input!.imageId! } : { kind: "host", id: hostId };
        try {
          await require("runScan", ref);
          const path = `/environments/${encodeURIComponent(hostId)}/${image ? `images/${encodeURIComponent(image)}/vulnerabilities/scan` : "vulnerabilities/scan"}`;
          const accepted = object(object(await request(path, "POST", image ? { origin: "manual" } : { scope: "in-use", origin: "manual" })).data);
          const activityId = id(accepted.activityId); o.accepted(hostId, activityId);
          return { hostId, activityId };
        } catch (error) {
          if (isCancelledError(error)) throw error;
          if (error instanceof VulnerabilityFailure && error.code === "vulnerability.busy" && error.activityId) {
            o.accepted(hostId, error.activityId);
            return { hostId, activityId: error.activityId, busy: true };
          }
          return { hostId, code: errorCode(error) };
        }
      }));
      if (!admissions.length || admissions.some((a) => !a.activityId)) throw new ScanAdmissionError(admissions);
      return admissions[0]!.activityId!;
    },
    async cancelScan(hostId, activityId) {
      await require("cancelScan", { kind: "host", id: hostId });
      await request(`/environments/${encodeURIComponent(hostId)}/activities/${encodeURIComponent(activityId)}/cancel`, "POST");
      o.accepted(hostId, activityId);
    },
  };
}
