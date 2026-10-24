// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original list-only discovery, bounded reads and demand regression coverage (#225, #223).

import { expect, it, vi } from "vitest";
import type { CodedActionRun } from "../message-code";
import { activity, activityFixture, deferred } from "./activity-reader.test-support";

it.each([0, 1, 100, 460])("cold discovery maps %s ordinary/runtime summaries with one list per host and zero details", async (count) => {
  const f = activityFixture();
  for (const host of ["0", "good", "bad"]) f.state.lists[host] = Array.from({ length: count }, (_, i) => ({ ...activity(host, `${host}-${i}`),
    ...(i % 2 ? { outcomeState: "unknown", cancelRequested: false, recovery: { unknownReason: "transport" } } : {}), stepTrace: [] }));
  for (const row of Object.values(f.state.lists).flat()) f.state.details[row.id] = { activity: row, stepTrace: row.stepTrace ?? [], messages: [], output: [], outputTruncated: false };
  const first = f.adapter.listActions();
  const second = f.adapter.listActions();
  expect(second).toBe(first);
  expect(await first).toHaveLength(count * 3);
  const lists = f.requests.filter((path) => /\/activities\?/.test(path));
  expect(lists).toHaveLength(3);
  expect(lists.every((path) => path.includes("limit=-1") && path.includes("includeSummary=true"))).toBe(true);
  expect(f.requests.filter((path) => /\/activities\/|\/runtime-runs\//.test(path))).toEqual([]);
});

it("snapshot and error refresh only their host; heartbeat performs zero reads", async () => {
  const f = activityFixture(); await f.adapter.listActions();
  const runs = vi.fn(); const health = vi.fn(); const stops = [f.adapter.subscribeActions(runs), f.adapter.subscribeActionReadHealth!(health)];
  const envelope = (type: string) => f.sources[0]!.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: "2026-10-24T00:00:00Z", activity: { type, environmentId: "good" } }), lastEventId: "" } as MessageEvent);
  vi.useFakeTimers();
  try {
    const before = f.requests.length;
    envelope("snapshot"); await vi.advanceTimersByTimeAsync(0);
    expect(f.requests.slice(before)).toHaveLength(1); expect(f.requests.at(-1)).toContain("/good/activities?");
    f.sources[0]!.onmessage?.({ data: JSON.stringify({ type: "heartbeat", timestamp: "2026-10-24T00:00:00Z" }), lastEventId: "" } as MessageEvent);
    await vi.advanceTimersByTimeAsync(0); expect(f.requests.slice(before)).toHaveLength(1);
    const held = deferred<void>(); const reached = deferred<void>();
    f.state.hold = async (path, response) => { if (path.endsWith("/good/activities")) { reached.resolve(); await held.promise; } return response; };
    envelope("error"); await reached.promise;
    expect(health.mock.lastCall?.[0].failures).toEqual([{ hostId: "good", reason: "unavailable" }]);
    expect(runs.mock.lastCall?.[0]).toHaveLength(2);
    held.resolve(); await vi.advanceTimersByTimeAsync(0);
    expect(health.mock.lastCall?.[0].failures).toEqual([]); expect(f.requests.slice(before)).toHaveLength(2);
  } finally { stops.forEach((stop) => stop()); vi.useRealTimers(); }
});

it("a cold partial list returns recovery rows before the browser deadline", async () => {
  const f = activityFixture();
  f.state.lists.bad = [{ ...activity("bad"), outcomeState: "unknown", cancelRequested: false, recovery: { disposition: "released", unknownReason: "transport" } }];
  const health = vi.fn(); const off = f.adapter.subscribeActionReadHealth!(health); vi.useFakeTimers();
  f.state.hold = async (path, response) => {
    if (path.endsWith("/bad/activities")) {
      await new Promise((resolve) => setTimeout(resolve, 30_000));
      response.headers.set("X-Docklet-Host-Reason", "agent.unavailable");
    }
    return response;
  };
  try {
    const pending = f.adapter.listActions(); let settled = false;
    void pending.then(() => { settled = true; });
    await vi.advanceTimersByTimeAsync(29_999); expect(settled).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    const runs = await pending;
    expect(runs.find((run) => run.id === "run-bad")).toMatchObject({ runtime: { released: true, unknownReason: "transport" }, output: [], log: [] });
    expect(health.mock.lastCall?.[0].failures).toEqual([{ hostId: "bad", reason: "unavailable" }]);
    await vi.advanceTimersByTimeAsync(10_000); expect(settled).toBe(true);
    expect(f.requests.filter((path) => /\/activities\//.test(path))).toEqual([]);
  } finally { off(); vi.useRealTimers(); }
});

it("opened detail shares reads, preserves output across lists and refreshes on changed remote summary", async () => {
  const f = activityFixture(); await f.adapter.listActions();
  const run = f.state.details["run-good"]!;
  run.output = [{ timestamp: run.activity.startedAt, stream: "stdout", text: "first output" }];
  const a = vi.fn(), b = vi.fn(); const published = vi.fn<(runs: CodedActionRun[]) => void>(); const off = f.adapter.subscribeActions(published);
  const stops = [f.adapter.observeActionDetail!("good", "run-good", a), f.adapter.observeActionDetail!("good", "run-good", b)];
  const refreshed = deferred<void>(), releaseRefresh = deferred<void>();
  vi.useFakeTimers();
  try {
    await vi.advanceTimersByTimeAsync(0);
    expect(a.mock.lastCall?.[0]).toMatchObject({ status: "ready", run: { output: [{ text: "first output" }] } });
    expect(b.mock.lastCall?.[0]).toEqual(a.mock.lastCall?.[0]);
    expect(f.requests.filter((path) => /\/activities\//.test(path))).toHaveLength(1);
    f.state.hold = async (path, response) => { if (path.endsWith("/good/activities/run-good")) { refreshed.resolve(); await releaseRefresh.promise; } return response; };
    run.output[0]!.text = "fresh output"; f.state.lists.good![0]!.progress = 75;
    f.sources[0]!.send(undefined, "good", "snapshot"); await refreshed.promise;
    expect(a.mock.lastCall?.[0]).toMatchObject({ status: "ready", refreshing: true, run: { output: [{ text: "first output" }] } });
    releaseRefresh.resolve(); await vi.advanceTimersByTimeAsync(0);
    expect(a.mock.lastCall?.[0]).toMatchObject({ status: "ready", run: { output: [{ text: "fresh output" }] } });
    expect(f.requests.filter((path) => /\/activities\//.test(path))).toHaveLength(2);
    expect(published.mock.lastCall?.[0].find((row) => row.id === "run-good")).toMatchObject({ progress: 75, output: [], log: [] });
    await f.adapter.listActions();
    expect(a.mock.lastCall?.[0]).toMatchObject({ run: { output: [{ text: "fresh output" }] } });
  } finally { releaseRefresh.resolve(); stops.forEach((stop) => stop()); off(); vi.useRealTimers(); }
});

it("targeted detail preserves recovery presentation absent from the generic detail contract", async () => {
  const f = activityFixture(); f.state.lists.good![0] = { ...activity("good"), outcomeState: "unknown", cancelRequested: false, recovery: { disposition: "released", unknownReason: "transport" } };
  await f.adapter.listActions(); const runs = vi.fn(); const off = f.adapter.subscribeActions(runs); vi.useFakeTimers();
  try {
    f.sources[0]!.send("run-good", "good", "message"); await vi.advanceTimersByTimeAsync(0);
    expect(runs.mock.lastCall?.[0].find((row: CodedActionRun) => row.id === "run-good")).toMatchObject({ runtime: { released: true, unknownReason: "transport" } });
  } finally { off(); vi.useRealTimers(); }
});

it("staggered initial snapshots stay within three lists per host even across a held source and open", async () => {
  const f = activityFixture(); const runs = vi.fn(); const off = f.adapter.subscribeActions(runs);
  const held = deferred<void>(), reached = deferred<void>(); let first = true;
  f.state.hold = async (path, response) => { if (first && path.endsWith("/bad/activities")) { first = false; reached.resolve(); await held.promise; } return response; };
  vi.useFakeTimers();
  try {
    const initial = f.adapter.listActions(); await reached.promise;
    f.sources[0]!.open(); f.sources[0]!.send(undefined, "good", "snapshot");
    await vi.advanceTimersByTimeAsync(0);
    expect(runs).not.toHaveBeenCalled();
    held.resolve(); await initial; await vi.advanceTimersByTimeAsync(0);
    for (const host of ["bad", "0"]) { f.sources[0]!.send(undefined, host, "snapshot"); await vi.advanceTimersByTimeAsync(0); }
    for (const host of ["0", "good", "bad"]) expect(f.requests.filter((path) => path.startsWith(`/api/environments/${host}/activities?`)), host).toHaveLength(3);
    expect(f.requests.filter((path) => /\/activities\/|\/runtime-runs\//.test(path))).toEqual([]);
  } finally { held.resolve(); off(); vi.useRealTimers(); }
});

it.each([false, true])("closing the final detail owner preserves only independently signalled summary work (notification=%s)", async (notification) => {
  const f = activityFixture(); const published = vi.fn(); const off = f.adapter.subscribeActions(published); await f.adapter.listActions();
  const held = deferred<Response>(), reached = deferred<Response>();
  f.state.details["run-good"] = { ...f.state.details["run-good"]!, activity: { ...f.state.details["run-good"]!.activity, status: "queued" } };
  f.state.hold = async (path, response) => path.endsWith("/good/activities/run-good") ? (reached.resolve(response), held.promise) : response;
  vi.useFakeTimers();
  const state = vi.fn(); const close = f.adapter.observeActionDetail!("good", "run-good", state);
  try {
    const response = await reached.promise; if (notification) f.sources[0]!.send("run-good", "good", "message"); close(); const count = published.mock.calls.length;
    held.resolve(response); await vi.advanceTimersByTimeAsync(1_000);
    expect(state).toHaveBeenCalledTimes(1);
    if (!notification) expect(published).toHaveBeenCalledTimes(count);
    expect(published.mock.lastCall?.[0].find((run: CodedActionRun) => run.id === "run-good")?.status).toBe(notification ? "queued" : "running");
  } finally { close(); held.resolve(jsonEmpty()); off(); vi.useRealTimers(); }
});

function jsonEmpty() { return new Response("{}"); }

it.each(["missing", "mapping", "wrong-host"] as const)("targeted contract validation retains the source-owned summary on %s", async (fault) => {
  const f = activityFixture(); await f.adapter.listActions();
  const runs = vi.fn(), health = vi.fn(); const off = f.adapter.subscribeActions(runs), stop = f.adapter.subscribeActionReadHealth!(health);
  vi.useFakeTimers();
  try {
    f.state.failure = fault; f.sources[0]!.send("run-bad", "bad", "message"); await vi.advanceTimersByTimeAsync(0);
    expect(health.mock.lastCall?.[0].failures).toEqual([{ hostId: "bad", reason: "invalid-data" }]);
    expect(runs.mock.lastCall?.[0].find((run: CodedActionRun) => run.id === "run-bad")).toMatchObject({ hostId: "bad", status: "running" });
  } finally { off(); stop(); vi.useRealTimers(); }
});
