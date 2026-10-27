// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: cf5aa27a8ca449d95c66dcd253f7a1a2e114f0f5
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original live vulnerability UI and contract regressions for issue #262.

import { scanSchedule } from "./security-settings";
import { vi } from "vitest";
import { resourceFixture } from "./resources.test-support";
import { createHttpAdapter } from "./http-adapter";

export const at = "2026-10-27T03:00:00Z";
export const counts = { critical: 2, high: 1, medium: 0, low: 0, unknown: 3, total: 6 };
export const terminalRun = { activityId: "history", environmentId: "0", origin: "manual", scope: "in-use", status: "completed", startedAt: at, finishedAt: at, targets: 1, scanned: 1, failed: 0, skipped: 0, ...counts, newCritical: 0, newHigh: 0, newFindings: [] };
export function securityFixture() {
  const inventory = resourceFixture(), original = inventory.fetcher.getMockImplementation()!;
  const state = { failureHost: "", busy: false, active: false, finished: false, delay: undefined as Promise<void> | undefined,
    settings: { setupCompleted: "true", featureVulnerabilityManagementEnabled: "true", vulnerabilityScanEnabled: "false", vulnerabilityScanSchedule: "", trivyIgnoreUnfixed: "true", vulnerabilityNotifyCritical: "false", vulnerabilityNotifyHigh: "false", vulnerabilityNotifyFailed: "false", effectiveTimeZone: "UTC" } as Record<string, string>,
    neverScanned: false, completedAt: at as string | undefined, findingCount: 6, duplicateFinding: false, findingReads: 0,
    refused: {} as Record<string, string>, denied: {} as Record<string, string>, hostNames: {} as Record<string, string>, hostStatuses: {} as Record<string, string>,
    progress: { targets: 10, scanned: 3, failed: 1, skipped: 1 },
    runs: [terminalRun], summaryStatus: "completed", scannedPlatform: "linux/amd64" as string | undefined, unknownCode: false, ignoreCritical: false };
  const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") } });
  const ok = (data: unknown) => json({ success: true, data });
  const page = (data: unknown[]) => json({ success: true, data, pagination: { totalItems: data.length } });
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com"), path = url.pathname, host = path.split("/")[3];
    if (path === "/api/environments") {
      const response = await original(String(input)), body = await response.json();
      body.data = body.data.map((h: { id: string; name: string }) => ({ ...h, name: state.hostNames[h.id] ?? h.name, ...(state.hostStatuses[h.id] ? { status: state.hostStatuses[h.id] } : {}) }));
      return json(body);
    }
    if (path === "/api/auth/mode") return ok({ mode: "none" });
    if (path === "/api/auth/me") return ok({ id: "operator", username: "operator", requiresPasswordChange: false, locale: "de" });
    if (path === "/api/capabilities") {
      const response = await original(String(input)), body = await response.json();
      body.data.push(...["getSecurity", "runScan", "setScanEnabled", "getScanSettings", "saveScanSettings", "cancelScan", "listActions"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" }, ...(operationId === "runScan" && state.denied[url.searchParams.get("id") ?? "0"] ? { available: false, disabledReasonCode: state.denied[url.searchParams.get("id") ?? "0"] } : {}) })));
      return json(body);
    }
    if (path === "/api/environments/0/settings" || path.endsWith("/settings/public")) {
      if (init?.method === "PUT") {
        const update = JSON.parse(String(init.body));
        try {
          if ("vulnerabilityScanSchedule" in update) {
            if (update.vulnerabilityScanSchedule === "") throw new Error("invalid vulnerability schedule");
            scanSchedule(update.vulnerabilityScanSchedule);
          }
          for (const [key, value] of Object.entries(update)) if (key !== "vulnerabilityScanSchedule" && value !== "true" && value !== "false") throw new Error("invalid boolean");
        } catch { return json({ status: 400, title: "Invalid scan settings" }, 400); }
        Object.assign(state.settings, update); return ok([]);
      }
      return json(Object.entries(state.settings).map(([key, value]) => ({ key, value })));
    }
    if (path === "/api/environments/0/jobs") return json({ jobs: [{ id: "vulnerability-scan", enabled: true, nextRun: state.settings.vulnerabilityScanSchedule ? "2026-10-28T03:00:00Z" : null }] });
    if (path.endsWith("/activities")) return page(state.active && host !== "0" ? [{ id: `scan-active-${host}`, environmentId: host, type: "vulnerability_scan", status: state.finished ? "success" : "running", startedAt: at, createdAt: at, progress: state.finished ? 100 : 35, resourceType: "environment", resourceId: "0", resourceName: "example", messageCode: "activity.started", ...(state.finished ? { endedAt: at } : {}) }] : []);
    if (path.endsWith("/cancel")) { state.finished = true; return ok({}); }
    if (/\/activities\/scan-active-[^/]+$/.test(path)) return ok({ activity: { id: `scan-active-${host}`, environmentId: host, type: "vulnerability_scan", status: state.finished ? "success" : "running", startedAt: at, createdAt: at, progress: state.finished ? 100 : 35, resourceType: "environment", resourceId: "0", resourceName: "example", messageCode: "activity.started" }, messages: [], stepTrace: [], output: [], outputTruncated: false });
    if (path.includes("/vulnerabilities")) {
      if (host === "second" && state.delay) await state.delay;
      if (host === state.failureHost) return json({ status: 503, code: "vulnerability.storage-unavailable" }, 503);
      if (path.endsWith("/summaries")) return ok({ summaries: { "sha256:user": state.neverScanned ? null : { imageId: "sha256:user", scanTime: at, status: state.summaryStatus, completedAt: state.completedAt, platform: "linux/arm64", scannedPlatform: state.scannedPlatform, summary: state.completedAt ? counts : undefined }, unused: null, dangling: null } });
      if (path.endsWith("/runs")) return ok(state.runs);
      if (path.endsWith("/overview")) return ok({ riskiestFindings: [{ vulnerabilityId: "CVE-2026-0001", knownExploited: true, epss: 0.23, risk: 9.4 }], prevalentFindings: [] });
      if (/\/vulnerabilities\/runs\/scan-active-[^/]+$/.test(path)) return ok({ ...terminalRun, activityId: `scan-active-${host}`, status: state.finished ? "completed" : "running", ...state.progress });
      if (path.endsWith("/all")) {
        state.findingReads++;
        const rows = state.neverScanned || !state.completedAt ? [] : Array.from({ length: state.findingCount }, (_, index) => ({ imageId: "sha256:user", imageName: "example:1.0.0", vulnerabilityId: `CVE-2026-${String(index + 1).padStart(4, "0")}`, pkgName: "example", installedVersion: "1.0", fixedVersion: "1.1", severity: ["UNKNOWN", "CRITICAL", "CRITICAL", "HIGH", "UNKNOWN", "UNKNOWN"][index % 6], kev: index === 0 ? true : undefined, epss: index === 0 ? 0.23 : undefined, risk: index === 0 ? 10 : 5.2, riskBand: index === 0 ? "critical" : "medium", cvss: { v3Score: 7.4 }, ignored: state.ignoreCritical && [1, 2].includes(index % 6) }));
        // Risk changes can move rows across page cuts; ID ordering keeps this fixed source stable.
        if (url.searchParams.get("sort") === "risk" && rows.length) rows.push(...rows.splice(0, state.findingReads % rows.length));
        if (state.duplicateFinding && rows[0]) rows.push({ ...rows[0] });
        const start = Number(url.searchParams.get("start") ?? 0), limit = Number(url.searchParams.get("limit") ?? 20);
        return json({ success: true, data: limit === -1 ? rows : rows.slice(start, start + Math.min(100, limit)), pagination: { totalItems: rows.length } });
      }
      if (path.endsWith("/scan")) {
        if (state.refused[host!]) { const status = state.refused[host!] === "agent.unavailable" ? 502 : 409; return json({ status, code: state.refused[host!] }, status); }
        state.active = true;
        return state.busy ? json({ status: 409, code: state.unknownCode ? "vulnerability.undeclared" : "vulnerability.busy", activityId: `scan-active-${host}` }, 409) : json({ success: true, data: { activityId: `scan-active-${host}` } }, 202);
      }
    }
    return original(String(input));
  });
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => inventory.source });
  return { adapter, state, fetcher, inventory, source: inventory.source };
}
