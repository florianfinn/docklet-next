// Source: docklet-next/issues/428
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original adapter-boundary scope, reason, retention and lifetime regression coverage. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags.

import { afterEach, expect, it, vi } from "vitest";
import { METRICS_FIRST_SAMPLE_MS } from "./metrics";
import { resourceFixture } from "./resources.test-support";
import type { MetricsSnapshot } from "@/domain/model";

const flush = async () => { for (let i = 0; i < 100; i++) await Promise.resolve(); };
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); });

it.each([false, true])("L3-h CPU unmount aborts through admission, releases its slot and remounts, Edge=%s", async (edge) => {
  const f = resourceFixture(); f.state.hostIds = ["first", "second", "third", "fourth", "fifth"]; f.state.isEdge = edge;
  await f.adapter.listHosts();
  const original = f.fetcher.getMockImplementation()!;
  const active: AbortSignal[] = [], aborted: AbortSignal[] = [];
  let hold = true;
  f.fetcher.mockImplementation(async (input, init?: RequestInit) => {
    if (hold && String(input).includes("sort=cpuUsage")) {
      const signal = init?.signal; if (signal) active.push(signal);
      await new Promise<void>((_resolve, reject) => {
        signal?.addEventListener("abort", () => { aborted.push(signal); reject(signal.reason); }, { once: true });
      });
    }
    return original(input);
  });
  const stop = f.adapter.subscribeMetrics(() => {}), last = f.adapter.subscribeMetrics(() => {});
  await vi.waitFor(() => expect(active).toHaveLength(5));
  const before = f.fetcher.mock.calls.length;
  const following = f.adapter.listContainers(); await flush();
  expect(f.fetcher.mock.calls.length).toBe(before);
  stop(); await flush(); expect(aborted).toHaveLength(0);
  last(); await flush();
  expect(aborted).toHaveLength(5); expect(active.every((signal) => signal.aborted)).toBe(true);
  expect((await following).length).toBeGreaterThan(0);
  hold = false; let snapshot: MetricsSnapshot | undefined;
  const remount = f.adapter.subscribeMetrics((value) => { snapshot = value; });
  await vi.waitFor(() => expect(snapshot?.containers["first~web"]?.cpu).toBe(2.5));
  remount();
});

it.each([false, true])("L3-h hidden aborts CPU and pauses timer, visibility refreshes once, Edge=%s", async (edge) => {
  vi.useFakeTimers(); const f = resourceFixture(); f.state.hostIds = ["first"]; f.state.isEdge = edge;
  const original = f.fetcher.getMockImplementation()!;
  let cpuReads = 0, aborted = false, hold = true, hidden = false;
  vi.spyOn(document, "hidden", "get").mockImplementation(() => hidden);
  f.fetcher.mockImplementation(async (input, init?: RequestInit) => {
    if (String(input).includes("sort=cpuUsage")) {
      cpuReads++;
      if (hold) await new Promise<void>((_resolve, reject) => init?.signal?.addEventListener("abort", () => { aborted = true; reject(init.signal?.reason); }, { once: true }));
    }
    return original(input);
  });
  const stop = f.adapter.subscribeMetrics(() => {});
  await vi.advanceTimersByTimeAsync(0); expect(cpuReads).toBe(1);
  hidden = true; document.dispatchEvent(new Event("visibilitychange")); await flush();
  expect(aborted).toBe(true);
  await vi.advanceTimersByTimeAsync(15_000); expect(cpuReads).toBe(1);
  hold = false; hidden = false; document.dispatchEvent(new Event("visibilitychange"));
  await vi.advanceTimersByTimeAsync(0); expect(cpuReads).toBe(2);
  stop();
});

it.each([false, true])("L3-h identity loss aborts CPU with subscribers still mounted, Edge=%s", async (edge) => {
  const f = resourceFixture(); f.state.hostIds = ["first"]; f.state.isEdge = edge;
  const original = f.fetcher.getMockImplementation()!;
  let entered = false, aborted = false;
  f.fetcher.mockImplementation(async (input, init?: RequestInit) => {
    if (String(input).includes("sort=cpuUsage")) {
      entered = true;
      await new Promise<void>((_resolve, reject) => init?.signal?.addEventListener("abort", () => { aborted = true; reject(init.signal?.reason); }, { once: true }));
    }
    if (String(input).endsWith("/backups/storage")) return new Response(null, { status: 401 });
    return original(input);
  });
  const stop = f.adapter.subscribeMetrics(() => {});
  await vi.waitFor(() => expect(entered).toBe(true));
  await f.adapter.getHubBackupStorage().catch(() => undefined);
  await vi.waitFor(() => expect(aborted).toBe(true)); stop();
});

it("R1D-3 an online host whose stats socket fails or stays silent before its first sample is reported unavailable, and a later sample clears it", async () => {
  vi.useFakeTimers(); const f = resourceFixture(); f.state.hostIds = ["first", "second"];
  let snapshot: MetricsSnapshot | undefined;
  const stop = f.adapter.subscribeMetrics((value) => { snapshot = value; });
  await vi.advanceTimersByTimeAsync(0);
  expect(snapshot?.unavailable ?? []).toEqual([]);
  // first: the socket fails before any sample; second: the socket stays silent.
  f.sockets.get("first")!.onerror?.({} as Event);
  await vi.advanceTimersByTimeAsync(0);
  expect(snapshot?.unavailable).toEqual(["first"]);
  await vi.advanceTimersByTimeAsync(METRICS_FIRST_SAMPLE_MS);
  expect([...(snapshot?.unavailable ?? [])].sort()).toEqual(["first", "second"]);
  f.sample("second");
  expect([snapshot?.unavailable, !!snapshot?.hosts.second]).toEqual([["first"], true]);
  stop();
});
