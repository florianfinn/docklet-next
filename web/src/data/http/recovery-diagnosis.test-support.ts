// Source: docklet-next/issues/389
// Revision: a331ec9df0c18670fd14d06d043cf58a4ef0fc80
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original adapter-boundary maintenance and Edge recovery diagnosis fixture.

import { vi } from "vitest";
import { createHttpAdapter } from "./http-adapter";
import { resourceFixture } from "./resources.test-support";
import { overview } from "./backups.test-support";

const at = "2026-11-11T12:00:00Z";
const headers = { "Content-Type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") };
const ok = (data: unknown) => Response.json({ success: true, data }, { headers });
export function recoveryFixture() {
  const resource = resourceFixture(), original = resource.fetcher.getMockImplementation()!;
  const state = { maintenance: false, edge: "online", revision: 1, critical: 1, eventTotal: 0, hostIds: ["first", "second"], hold: new Set<string>() };
  const pending: Array<{ path: string; release: () => void; aborted: boolean }> = [];
  const requests: Array<{ path: string; start: number }> = [];
  const rows = () => state.hostIds.map((id) => ({ id, name: id, apiUrl: "http://agent.example.com", enabled: true, status: id === "second" ? state.edge : "online", isEdge: id === "second", registrationRevision: state.revision }));
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com"), path = url.pathname;
    requests.push({ path, start: Date.now() });
    let answer: Response;
    if (path === "/api/auth/mode") answer = ok({ mode: "none" });
    else if (path === "/api/auth/me" || path === "/api/auth/login") {
      const user = { id: "operator", username: "operator", requiresPasswordChange: false, isAdmin: true, locale: "en-US" };
      answer = ok(path.endsWith("/login") ? { user } : user);
    } else if (path === "/api/backups/maintenance") answer = Response.json({ active: state.maintenance, operation: state.maintenance ? "backup" : undefined }, { headers });
    else if (path.endsWith("/test")) answer = ok({ status: "online" });
    else if (path === "/api/environments/first" && init?.method === "PUT") answer = ok(rows()[0]);
    else if (path.endsWith("/updater/system-image-hints")) answer = ok({ images: [] });
    else if (path === "/api/environments") answer = Response.json({ success: true, data: rows(), pagination: { totalItems: state.hostIds.length } }, { headers });
    else if (path === "/api/capabilities") {
      answer = await original(input); const body = await answer.json();
      const extra = ["updateHost", "connectHost", "reconnectHost", "listSystemComponents","getSecurity", "listBackupOverviews", "getBackupOverview", "listUpdatePolicies", "saveSettings", "listEvents"];
      body.data.push(...extra.map((operationId) => ({ operationId, available: true, resourceRef: { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" } })));
      answer = Response.json(body, { headers });
    } else if (path.endsWith("/settings/public")) answer = Response.json([{ key: "featureVulnerabilityManagementEnabled", value: "true" }], { headers });
    else if (path.endsWith("/volumes/backups/overview")) answer = ok(overview);
    else if (path.endsWith("/volumes/binds")) answer = ok([]);
    else if (path.endsWith("/images/vulnerabilities/active-summaries")) answer = ok({ images: [{ imageId: "sha256:example", inUse: true, lastCompletedScanAt: at, latestAttemptStatus: "completed", summary: { critical: state.critical, high: 0, medium: 0, low: 0, unknown: 0, total: state.critical } }] });
    else if (path.endsWith("/updater/policies")) answer = ok([]);
    else if (path.endsWith("/settings")) answer = Response.json([{ key: "effectiveTimeZone", value: "UTC" }], { headers });
    else if (path === "/api/events") answer = Response.json({ success: true, data: Array.from({ length: state.eventTotal }, (_, i) => ({ id: `event-${i}`, type: "project.deploy", messageCode: "project.deploy", severity: "success", timestamp: at, createdAt: at, title: "Example" })), pagination: { totalItems: state.eventTotal } }, { headers });
    else answer = await original(input);
    if (state.hold.has(path)) {
      await new Promise<void>((resolve, reject) => {
        const abort = () => { entry.aborted = true; reject(init?.signal?.reason); };
        const entry = { path, aborted: false, release: () => { init?.signal?.removeEventListener("abort", abort); resolve(); } };
        pending.push(entry);
        if (init?.signal?.aborted) abort(); else init?.signal?.addEventListener("abort", abort, { once: true });
      });
    }
    return answer;
  });
  const source = resource.source;
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => source });
  const send = () => source.onmessage?.({ data: JSON.stringify({ channel: "environments", timestamp: at, environment: { environments: rows() } }), lastEventId: "" } as MessageEvent);
  return { adapter, state, pending, requests, source, send, fetcher, releasePath: (path: string) => { state.hold.delete(path); pending.filter(p => p.path === path).forEach(p => p.release()); }, release: () => { state.hold.clear(); pending.forEach((p) => p.release()); } };
}
