// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: cf5aa27a8ca449d95c66dcd253f7a1a2e114f0f5
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original live vulnerability UI and contract regressions for issue #262.

import type { DockletAdapter } from "../adapter";
import { unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import type { ScanSettings, ScanSettingsInput } from "@/domain/security";
import type { ScanSchedule } from "@/domain/scan-schedule";
import { array, boolean, choice, date, invalid, object, string } from "./vulnerability-decoders";

type Request = (path: string, method?: string, body?: object, envelope?: boolean) => Promise<unknown>;
const HUB: ResourceRef = { kind: "hub", id: "0" };
export function scanSchedule(v: unknown): ScanSchedule {
  if (v === "") return { mode: "manual" };
  let raw: unknown;
  try { raw = typeof v === "string" ? JSON.parse(v) : v; } catch { invalid(); }
  const o = object(raw), mode = choice(o.mode, ["daily", "weekly"]), time = string(o.time, 5);
  if (!/^([01][0-9]|2[0-3]):[0-5][0-9]$/.test(time) || Object.keys(o).some((k) => !["mode", "time", "day"].includes(k))) invalid();
  if (mode === "weekly") return { mode, time, day: choice(o.day, ["sun", "mon", "tue", "wed", "thu", "fri", "sat"]) };
  if (o.day !== undefined && o.day !== "") invalid();
  return { mode, time };
}
export function settingsValues(v: unknown): Record<string, string> {
  return Object.fromEntries(array(v).map((raw) => { const o = object(raw); return [string(o.key, 256), string(o.value)]; }));
}
export function createSecuritySettings(request: Request, capability: (op: string, ref: ResourceRef) => Promise<CapabilityDescriptor>, changed: () => void): Required<Pick<DockletAdapter, "getScanSettings" | "saveScanSettings" | "setScanEnabled">> {
  const require = async (operation: string) => { const c = await capability(operation, HUB); if (!c.available) throw unavailable(operation, HUB, c.disabledReasonCode); };
  async function write(value: Record<string, string>) { await request("/environments/0/settings", "PUT", value); changed(); }
  return {
    async getScanSettings(): Promise<ScanSettings> {
      await require("getScanSettings");
      const values = settingsValues(await request("/environments/0/settings", "GET", undefined, false));
      const b = (key: string) => { if (values[key] !== "true" && values[key] !== "false") invalid(); return values[key] === "true"; };
      const timeZone = string(values.effectiveTimeZone, 256);
      try { new Intl.DateTimeFormat("en", { timeZone }); } catch { invalid(); }
      const result: ScanSettings = { enabled: b("featureVulnerabilityManagementEnabled"), automatic: b("vulnerabilityScanEnabled"), schedule: scanSchedule(values.vulnerabilityScanSchedule), ignoreUnfixed: b("trivyIgnoreUnfixed"), notifyCritical: b("vulnerabilityNotifyCritical"), notifyHigh: b("vulnerabilityNotifyHigh"), notifyFailed: b("vulnerabilityNotifyFailed"), timeZone };
      // Jobs have a separate permission. Missing projection never becomes a local recurrence.
      try {
        const jobs = object(await request("/environments/0/jobs", "GET", undefined, false));
        const job = array(jobs.jobs).map(object).find((j) => j.id === "vulnerability-scan");
        if (!job) invalid();
        boolean(job.enabled);
        result.nextRun = job.nextRun == null ? undefined : date(job.nextRun);
      } catch { result.projectionUnavailable = true; }
      return result;
    },
    async saveScanSettings(v: ScanSettingsInput) {
      await require("saveScanSettings");
      const schedule = v.schedule.mode === "manual" ? "" : JSON.stringify(scanSchedule(v.schedule));
      await write({ featureVulnerabilityManagementEnabled: String(boolean(v.enabled)), vulnerabilityScanEnabled: String(boolean(v.automatic)), vulnerabilityScanSchedule: schedule, trivyIgnoreUnfixed: String(boolean(v.ignoreUnfixed)), vulnerabilityNotifyCritical: String(boolean(v.notifyCritical)), vulnerabilityNotifyHigh: String(boolean(v.notifyHigh)), vulnerabilityNotifyFailed: String(boolean(v.notifyFailed)) });
    },
    async setScanEnabled(enabled) { await require("setScanEnabled"); await write({ featureVulnerabilityManagementEnabled: String(boolean(enabled)) }); },
  };
}
