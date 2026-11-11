// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original consumer proofs for the Dashboard's active vulnerability counts (S1b route, AUTO-4, AUTO-6, R1C-4).

import { afterEach, expect, it, vi } from "vitest";

import type { ContainerInfo } from "@/domain/model";
import { activeOverview } from "@/features/dashboard/active-images";
import { INVENTORY_RECOVERY_MS, inventoryRecoveryInterval } from "../inventory-recovery";
import { securityFixture } from "./vulnerabilities.test-support";

const summary = (critical: number, high: number) => ({ critical, high, medium: 1, low: 0, unknown: 0, total: critical + high + 1 });

/** The security fixture's hub plus the active-summaries route of each host. */
function active() {
  const f = securityFixture();
  const original = f.fetcher.getMockImplementation()!;
  const answers: Record<string, unknown> = {
    first: { images: [
      { imageId: "sha256:aaa", inUse: true, lastCompletedScanAt: "2026-11-11T10:00:00Z", latestAttemptStatus: "completed", summary: summary(1, 2) },
      { imageId: "sha256:bbbbbbbbbbbbbbbb", inUse: false, lastCompletedScanAt: "2026-11-11T10:00:00Z", latestAttemptStatus: "completed", summary: summary(0, 1) },
      { imageId: "sha256:ccc", inUse: true, lastCompletedScanAt: null, latestAttemptStatus: "failed", latestAttemptErrorCode: "vulnerability.scan-failed", summary: summary(0, 0) },
    ] },
  };
  const sent: Array<{ path: string; method: string }> = [];
  f.fetcher.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (!url.pathname.endsWith("/images/vulnerabilities/active-summaries")) return original(input, init);
    const host = url.pathname.split("/")[3]!;
    sent.push({ path: url.pathname, method: init?.method ?? "GET" });
    if (host === f.state.failureHost) return new Response(JSON.stringify({ status: 503, code: "vulnerability.storage-unavailable" }), { status: 503, headers: { "Content-Type": "application/json" } });
    return Response.json({ success: true, data: answers[host] ?? { images: [] } });
  });
  return { f, sent, answers };
}

it("reads each host's active counts with one POST and no catalogue, summaries or findings reads", async () => {
  const { f, sent } = active();
  const result = await f.adapter.getActiveSecurity();
  const paths = f.fetcher.mock.calls.map(([input]) => new URL(String(input), "https://hub.example.com").pathname);
  expect(sent.length > 0 && sent.every((s) => s.method === "POST")).toBe(true);
  expect(paths.some((p) => /\/images(\/vulnerabilities\/summaries|\/vulnerabilities$|$)|\/vulnerabilities\/findings/.test(p))).toBe(false);
  const first = result.images.filter((i) => i.hostId === "first");
  expect(first.map((i) => [i.id, i.inUse, i.counts.critical + i.counts.high, i.scannedAt])).toEqual([
    ["sha256:aaa", true, 3, "2026-11-11T10:00:00Z"],
    ["sha256:bbbbbbbbbbbbbbbb", false, 1, "2026-11-11T10:00:00Z"],
    // R1C-4: no completed scan yet → not scanned.
    ["sha256:ccc", true, 0, ""],
  ]);
});

it("a failed host keeps its last counts as new unavailable objects and reports its code", async () => {
  const { f } = active();
  const before = await f.adapter.getActiveSecurity();
  const previous = Object.freeze(before.images.filter((i) => i.hostId === "first").map((i) => Object.freeze(i)));
  f.state.failureHost = "first";
  const after = await f.adapter.getActiveSecurity();
  const retained = after.images.filter((i) => i.hostId === "first");
  expect(retained.map((i) => i.id)).toEqual(previous.map((i) => i.id));
  expect(retained.every((i) => i.unavailable === true)).toBe(true);
  expect(retained.some((i) => previous.includes(i))).toBe(false);
  expect(after.unavailable.map((u) => u.hostId)).toEqual(["first"]);
  // A second failure still shows the last good counts, not the previous unavailable copy only.
  const again = await f.adapter.getActiveSecurity();
  expect(again.images.filter((i) => i.hostId === "first").map((i) => i.counts.high)).toEqual(previous.map((i) => i.counts.high));
});

it("AUTO-6 the Dashboard names images by their containers, keeps today's totals and names an unused image by its short id", () => {
  const container = (id: string, imageId: string, state: string, stackId?: string) => ({ id, name: id, hostId: "first", imageId, state, stackId, origin: "compose", image: { repository: "example.com/app", tag: "1" } }) as unknown as ContainerInfo;
  const overview = activeOverview({ enabled: true, unavailable: [], images: [
    { id: "sha256:aaa", hostId: "first", inUse: true, counts: { critical: 1, high: 2, medium: 1, low: 0, unknown: 0 }, scannedAt: "x" },
    { id: "sha256:bbbbbbbbbbbbbbbb", hostId: "first", inUse: false, counts: { critical: 0, high: 1, medium: 1, low: 0, unknown: 0 }, scannedAt: "x" },
  ] }, [container("web", "first~sha256:aaa", "running", "app"), container("other", "first~sha256:aaa", "exited")]);
  expect(overview.images.map((i) => [i.ref, i.usage, i.stackId])).toEqual([["example.com/app:1", "running", "app"], ["bbbbbbbbbbbb", "unused", undefined]]);
  expect(overview.images.reduce((n, i) => n + i.counts.critical + i.counts.high, 0)).toBe(4);
});

afterEach(() => { vi.restoreAllMocks(); });

it("the read is tagged: an offline host is skipped without a request and keeps its last counts unavailable", async () => {
  const { f, sent } = active();
  await f.adapter.getActiveSecurity();
  const before = sent.filter((s) => s.path.startsWith("/api/environments/first/")).length;
  f.state.hostStatuses.first = "offline";
  // The catalogue read after its stale time reports the host offline.
  const now = Date.now();
  vi.spyOn(Date, "now").mockReturnValue(now + 11_000);
  const after = await f.adapter.getActiveSecurity();
  expect(sent.filter((s) => s.path.startsWith("/api/environments/first/")).length).toBe(before);
  expect(after.images.filter((i) => i.hostId === "first").every((i) => i.unavailable === true)).toBe(true);
  expect(after.unavailable.map((u) => [u.hostId, u.code])).toEqual([["first", "agent.unavailable"]]);
});

it("R1D-1 the Dashboard joins active counts to the adapter's mapped containers: name, running state and stack of the real producer", async () => {
  const { f, answers } = active();
  answers.first = { images: [
    { imageId: "sha256:user", inUse: true, lastCompletedScanAt: "2026-11-11T10:00:00Z", latestAttemptStatus: "completed", summary: summary(1, 0) },
    { imageId: "sha256:dddddddddddddddd", inUse: false, lastCompletedScanAt: "2026-11-11T10:00:00Z", latestAttemptStatus: "completed", summary: summary(0, 0) },
  ] };
  const containers = await f.adapter.listContainers();
  const overview = activeOverview(await f.adapter.getActiveSecurity(), containers);
  expect(overview.images.filter((i) => i.hostId === "first").map((i) => [i.ref, i.usage, i.stackId])).toEqual([
    ["example:1.0.0", "running", "first~project"],
    ["dddddddddddd", "unused", undefined],
  ]);
});

it("R1D-4 a transient failure of an online host schedules recovery; a skipped offline host does not", async () => {
  const { f } = active();
  await f.adapter.getActiveSecurity();
  f.state.failureHost = "first";
  const failed = await f.adapter.getActiveSecurity();
  const hosts = await f.adapter.listHosts();
  expect(inventoryRecoveryInterval(failed, hosts.map((h) => ({ ...h, status: "online" as const })))).toBe(INVENTORY_RECOVERY_MS);
  expect(inventoryRecoveryInterval(failed, hosts.map((h) => ({ ...h, status: "offline" as const })))).toBe(false);
});
