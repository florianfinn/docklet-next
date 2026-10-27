// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: cf5aa27a8ca449d95c66dcd253f7a1a2e114f0f5
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original live vulnerability UI and contract regressions for issue #262.

import { vi } from "vitest";
import { resourceFixture } from "./resources.test-support";
import { createHttpAdapter } from "./http-adapter";

export const at = "2026-10-27T03:00:00Z";
export const counts = { critical: 2, high: 1, medium: 0, low: 0, unknown: 3, total: 6 };
export const terminalRun = { activityId: "history", environmentId: "0", origin: "manual", scope: "in-use", status: "completed", startedAt: at, finishedAt: at, targets: 1, scanned: 1, failed: 0, skipped: 0, ...counts, newCritical: 0, newHigh: 0, newFindings: [] };
export function securityFixture() {
  const inventory = resourceFixture(), original = inventory.fetcher.getMockImplementation()!;
  const state = { failureHost: "", busy: false, active: false, finished: false, delay: undefined as Promise<void> | undefined,
    settings: { setupCompleted: "true", featureVulnerabilityManagementEnabled: "true", vulnerabilityScanEnabled: "false", vulnerabilityScanSchedule: '{"mode":"daily","time":"03:00"}', trivyIgnoreUnfixed: "true", vulnerabilityNotifyCritical: "false", vulnerabilityNotifyHigh: "false", vulnerabilityNotifyFailed: "false", effectiveTimeZone: "UTC" } as Record<string, string>,
    runs: [terminalRun], summaryStatus: "completed", scannedPlatform: "linux/amd64" as string | undefined, unknownCode: false, ignoreCritical: false };
  const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") } });
  const ok = (data: unknown) => json({ success: true, data });
  const page = (data: unknown[]) => json({ success: true, data, pagination: { totalItems: data.length } });
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com"), path = url.pathname, host = path.split("/")[3];
    if (path === "/api/auth/mode") return ok({ mode: "none" });
    if (path === "/api/auth/me") return ok({ id: "operator", username: "operator", requiresPasswordChange: false, locale: "de" });
    if (path === "/api/capabilities") {
      const response = await original(String(input)), body = await response.json();
      body.data.push(...["getSecurity", "runScan", "setScanEnabled", "getScanSettings", "saveScanSettings", "cancelScan", "listActions"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" } })));
      return json(body);
    }
    if (path === "/api/environments/0/settings" || path.endsWith("/settings/public")) {
      if (init?.method === "PUT") { Object.assign(state.settings, JSON.parse(String(init.body))); return ok([]); }
      return json(Object.entries(state.settings).map(([key, value]) => ({ key, value })));
    }
    if (path === "/api/environments/0/jobs") return json({ jobs: [{ id: "vulnerability-scan", enabled: true, nextRun: "2026-10-28T03:00:00Z" }] });
    if (path.endsWith("/activities")) return page(state.active && host !== "0" ? [{ id: `scan-active-${host}`, environmentId: host, type: "vulnerability_scan", status: state.finished ? "success" : "running", startedAt: at, createdAt: at, progress: state.finished ? 100 : 35, resourceType: "environment", resourceId: "0", resourceName: "example", messageCode: "activity.started", ...(state.finished ? { endedAt: at } : {}) }] : []);
    if (path.endsWith("/cancel")) { state.finished = true; return ok({}); }
    if (/\/activities\/scan-active-[^/]+$/.test(path)) return ok({ activity: { id: `scan-active-${host}`, environmentId: host, type: "vulnerability_scan", status: state.finished ? "success" : "running", startedAt: at, createdAt: at, progress: state.finished ? 100 : 35, resourceType: "environment", resourceId: "0", resourceName: "example", messageCode: "activity.started" }, messages: [], stepTrace: [], output: [], outputTruncated: false });
    if (path.includes("/vulnerabilities")) {
      if (host === "second" && state.delay) await state.delay;
      if (host === state.failureHost) return json({ status: 503, code: "vulnerability.storage-unavailable" }, 503);
      if (path.endsWith("/summaries")) return ok({ summaries: { "sha256:user": { imageId: "sha256:user", scanTime: at, status: state.summaryStatus, completedAt: at, platform: "linux/arm64", scannedPlatform: state.scannedPlatform, summary: counts }, unused: null, dangling: null } });
      if (path.endsWith("/runs")) return ok(state.runs);
      if (path.endsWith("/overview")) return ok({ riskiestFindings: [{ vulnerabilityId: "CVE-2026-0001", knownExploited: true, epss: 0.23, risk: 9.4 }], prevalentFindings: [] });
      if (path.endsWith("/all")) return page(["UNKNOWN", "CRITICAL", "CRITICAL", "HIGH", "UNKNOWN", "UNKNOWN"].map((severity, index) => ({ imageId: "sha256:user", imageName: "example:1.0.0", vulnerabilityId: `CVE-2026-000${index + 1}`, pkgName: "example", installedVersion: "1.0", fixedVersion: "1.1", severity, kev: index === 0 ? true : undefined, epss: index === 0 ? 0.23 : undefined, risk: index === 0 ? 10 : 5.2, riskBand: index === 0 ? "critical" : "medium", cvss: { v3Score: 7.4 }, ignored: state.ignoreCritical && severity === "CRITICAL" })));
      if (path.endsWith("/scan")) {
        state.active = true;
        return state.busy ? json({ status: 409, code: state.unknownCode ? "vulnerability.undeclared" : "vulnerability.busy", activityId: `scan-active-${host}` }, 409) : json({ success: true, data: { activityId: `scan-active-${host}` } }, 202);
      }
    }
    return original(String(input));
  });
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => inventory.source });
  return { adapter, state, fetcher, inventory, source: inventory.source };
}
