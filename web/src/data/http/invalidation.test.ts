// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original resource inventory contract tests.

import { afterEach, expect, it, vi } from "vitest";
import type { Topic } from "@/data/adapter";
import { at, resourceFixture } from "./resources.test-support";

afterEach(() => vi.useRealTimers());

it("budgets at most 78 HTTP reads for an idle stacks page with seven ordinary SSE envelopes in one minute", async () => {
  vi.useFakeTimers(); vi.setSystemTime(new Date(at));
  const f = resourceFixture(); f.state.dynamicObservation = true;
  const pending: Promise<unknown>[] = [];
  const stopMetrics = f.adapter.subscribeMetrics(() => {});
  const changes: Topic[][] = [];
  const stop = f.adapter.subscribeChanges((topics) => {
    changes.push(topics);
    if (topics.includes("hosts")) pending.push(f.adapter.listHosts());
    if (topics.includes("stacks")) pending.push(f.adapter.listStacks());
  });
  try {
    await Promise.all([f.adapter.listHosts(), f.adapter.listStacks()]);
    await vi.advanceTimersByTimeAsync(0);
    await Promise.all(pending.splice(0));
    const initial = f.fetcher.mock.calls.length;
    expect(initial).toBe(8); // One capability, one shared host list and six inventory reads.
    for (let second = 0; second < 60; second += 10) {
      const payload = second % 20 === 0 ? { channel: "activities", activity: { type: "updated" } } : { channel: "environments", environment: { type: "snapshot", environments: f.state.hostIds.map((id) => ({ id, name: id, apiUrl: "http://agent.example.com", enabled: true, status: "online", isEdge: false, lastSeen: `2026-10-17T12:00:${second}Z` })) } };
      f.source.onmessage?.({ data: JSON.stringify({ ...payload, timestamp: at }), lastEventId: "" } as MessageEvent);
      await Promise.all(pending.splice(0));
      await vi.advanceTimersByTimeAsync(10_000);
      await Promise.all(pending.splice(0));
    }
    f.source.onmessage?.({ data: JSON.stringify({ channel: "events", eventLog: { type: "created" }, timestamp: at }), lastEventId: "" } as MessageEvent);
    // 12 initial + 6 snapshots + 12 polls × (hosts + two container lists + two fresh observation notices).
    expect(f.fetcher.mock.calls.length).toBeLessThanOrEqual(78);
    expect(f.fetcher.mock.calls.filter(([url]) => /\/(projects|containers)\?/.test(String(url)))).toHaveLength(30);
    expect(changes.every((topics) => topics.every((t) => t === "events" || t === "hosts"))).toBe(true);
  } finally { stop(); stopMetrics(); }
});

it("invalidates inventory on connection/state changes and caches capabilities per resource and session", async () => {
  const f = resourceFixture();
  const topics = vi.fn(); const stop = f.adapter.subscribeChanges(topics);
  try {
    await f.adapter.listHosts();
    await Promise.all([f.adapter.getCapability("listImages"), f.adapter.getCapability("listVolumes")]);
    expect(f.fetcher.mock.calls.filter(([url]) => String(url).includes("/capabilities"))).toHaveLength(1);
    const ref = { kind: "host" as const, id: "first" };
    await Promise.all([f.adapter.getCapability("listImages", ref), f.adapter.getCapability("listVolumes", ref)]);
    expect(f.fetcher.mock.calls.filter(([url]) => String(url).includes("/capabilities"))).toHaveLength(2);
    f.source.onmessage?.({ data: JSON.stringify({ timestamp: at, channel: "environments", environment: { type: "snapshot", environments: f.state.hostIds.map((id) => ({ id, name: id, status: id === "second" ? "offline" : "online", apiUrl: "http://agent.example.com", enabled: true, isEdge: false })) } }), lastEventId: "" } as MessageEvent);
    expect(topics).toHaveBeenLastCalledWith(["hosts", "stacks", "containers", "images", "volumes", "topology", "security"]);
    await f.adapter.getCapability("listImages");
    expect(f.fetcher.mock.calls.filter(([url]) => String(url).includes("/capabilities"))).toHaveLength(3);
    const original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input) => String(input).endsWith("/auth/me") ? new Response(JSON.stringify({ success: true, data: { id: "operator", username: "operator", requiresPasswordChange: false } })) : original(input));
    await f.adapter.getCurrentUser(); await f.adapter.getCapability("listImages");
    expect(f.fetcher.mock.calls.filter(([url]) => String(url).includes("/capabilities"))).toHaveLength(4);
  } finally { stop(); }
});

it("retries failed capability reads instead of caching an outage", async () => {
  const f = resourceFixture(); f.fetcher.mockRejectedValueOnce(new Error("offline"));
  expect((await f.adapter.getCapability("listImages")).available).toBe(false);
  expect((await f.adapter.getCapability("listImages")).available).toBe(true);
});
