// Source: docklet-next/issues/102
// Revision: dd403606fbbf763798d43f4ee854adcb105e01b1
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original activity isolation, recovery and ordering regressions.

import { CancelledError } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import type { ActionReadHealth } from "../adapter";
import type { CodedActionRun } from "../message-code";
import { activity, activityFixture, deferred, detail, json } from "./activity-reader.test-support";

const failures = ["network", "page500", "page502", "page403", "detail404", "missing", "mapping", "wrong-host", "pagination"] as const;
for (const isEdge of [false, true]) for (const reverse of [false, true]) {
  it.each(failures)(`isolates %s (Edge fixture=${isEdge}, reverse=${reverse})`, async (mode) => {
    const f = activityFixture(isEdge, reverse);
    f.state.failure = mode;
    const health = vi.fn<(h: ActionReadHealth) => void>();
    const stop = f.adapter.subscribeActionReadHealth!(health);
    try {
      const runs = await f.adapter.listActions();
      expect(runs.map((r) => [r.id, r.hostId, r.status])).toEqual([["run-good", "good", "running"]]);
      expect(health.mock.lastCall?.[0].failures).toEqual([{ hostId: "bad", reason: mode === "page403" ? "permission" : ["missing", "mapping", "wrong-host"].includes(mode) ? "invalid-data" : "unavailable" }]);
      expect(f.requests.length).toBeLessThanOrEqual(7);
    } finally { stop(); }
  });
}
it.each(["page401", "hub401"])("rejects global authentication failure %s and preserves unauthorized signal", async (mode) => {
  const f = activityFixture();
  const health = vi.fn();
  const unauthorized = vi.fn();
  window.addEventListener("docklet:unauthorized", unauthorized);
  const stop = f.adapter.subscribeActionReadHealth!(health);
  try {
    if (mode === "page401") f.state.failure = "page401"; else f.state.hubStatus = 401;
    await expect(f.adapter.listActions()).rejects.toMatchObject({ outcome: { kind: "error", status: 401 } });
    expect(unauthorized).toHaveBeenCalled();
    expect(health.mock.lastCall?.[0].failures[0]).toEqual({ hostId: undefined, reason: "authentication" });
  } finally { stop(); window.removeEventListener("docklet:unauthorized", unauthorized); }
});
it("retains unavailable observations without terminalizing; successful empty and host removal delete rows", async () => {
  const f = activityFixture();
  const health = vi.fn(); const stop = f.adapter.subscribeActionReadHealth!(health);
  const runs = vi.fn(); const unsubscribe = f.adapter.subscribeActions(runs);
  try {
    await f.adapter.listActions();
    f.state.failure = "page502"; f.state.lists.good = [];
    expect((await f.adapter.listActions()).map((r) => [r.id, r.status, r.finishedAt])).toEqual([["run-bad", "running", undefined]]);
    expect(health.mock.lastCall?.[0].failures).toHaveLength(1);
    expect(runs.mock.lastCall?.[0].map((r: CodedActionRun) => [r.id, r.status])).toEqual([["run-bad", "running"]]);
    f.state.failure = ""; f.state.lists.bad = [];
    expect(await f.adapter.listActions()).toEqual([]);
    expect(health.mock.lastCall?.[0].failures).toEqual([]);
    f.state.lists.bad = [activity("bad")]; await f.adapter.listActions();
    f.state.hosts = ["good"];
    f.sources[0]!.environments();
    await vi.waitFor(() => expect(runs.mock.lastCall?.[0]).toEqual([]));
  } finally { unsubscribe(); stop(); }
});
it("keeps valid details from an incomplete source and rejects colliding or misattributed identities", async () => {
  const f = activityFixture();
  f.state.lists.bad!.push(activity("bad", "valid-bad"));
  f.state.details["valid-bad"] = detail("bad", "valid-bad");
  f.state.hold = async (path, response) => path.endsWith("/bad/activities/run-bad") ? json({}) : response;
  expect((await f.adapter.listActions()).map((r) => r.id).sort()).toEqual(["run-good", "valid-bad"]);
  f.state.hold = undefined;
  f.state.lists.bad = [activity("bad", "run-good")];
  const health = vi.fn(); const stop = f.adapter.subscribeActionReadHealth!(health);
  expect((await f.adapter.listActions()).find((r) => r.id === "run-good")?.hostId).toBe("good");
  expect(health.mock.lastCall?.[0].failures.some((v: { hostId: string }) => v.hostId === "bad")).toBe(true);
  stop();
});
it("recovers a failed direct/Edge-tagged source only after a real read and does not refetch in a loop", async () => {
  for (const edge of [false, true]) {
    const f = activityFixture(edge); f.state.failure = "page502";
    const runs = vi.fn(); const health = vi.fn();
    const stops = [f.adapter.subscribeActions(runs), f.adapter.subscribeActionReadHealth!(health)];
    try {
      f.sources[0]!.open();
      await vi.waitFor(() => expect(runs.mock.lastCall?.[0]).toHaveLength(1));
      const count = f.requests.length;
      await new Promise((resolve) => setTimeout(resolve, 20));
      expect(f.requests).toHaveLength(count);
      f.state.failure = "";
      expect(health.mock.lastCall?.[0].failures).toHaveLength(1);
      f.sources[0]!.open();
      await vi.waitFor(() => expect(runs.mock.lastCall?.[0]).toHaveLength(2));
      expect(health.mock.lastCall?.[0].failures).toEqual([]);
      expect(f.requests.length - count).toBeLessThanOrEqual(6);
    } finally { stops.forEach((stop) => stop()); }
  }
});
it("a slow old full read cannot overwrite a newer targeted detail while another source fails", async () => {
  const f = activityFixture(); const runs = vi.fn<(r: CodedActionRun[]) => void>();
  const stop = f.adapter.subscribeActions(runs);
  try {
    f.sources[0]!.open(); await vi.waitFor(() => expect(runs).toHaveBeenCalled());
    const held = deferred<Response>(); let captured: Response | undefined;
    f.state.hold = async (path, response) => {
      if (path.endsWith("/good/activities/run-good")) { captured = response; return held.promise; }
      return response;
    };
    f.state.failure = "page502"; const older = f.adapter.listActions();
    await vi.waitFor(() => expect(captured !== undefined).toBe(true));
    f.state.hold = undefined; f.state.details["run-good"]!.activity.status = "queued";
    f.sources[0]!.send("run-good", "good");
    await vi.waitFor(() => expect(runs.mock.lastCall?.[0].find((r) => r.id === "run-good")?.status).toBe("queued"));
    held.resolve(captured!);
    await older;
    expect(runs.mock.lastCall?.[0]).toHaveLength(2);
    expect(runs.mock.lastCall?.[0].find((r) => r.id === "run-good")?.status).toBe("queued");
  } finally { stop(); }
});
it("targeted detail failure is visible; a single detail success cannot clear an incomplete source", async () => {
  const f = activityFixture(); const runs = vi.fn(); const health = vi.fn();
  const stops = [f.adapter.subscribeActions(runs), f.adapter.subscribeActionReadHealth!(health)];
  try {
    f.sources[0]!.open(); await vi.waitFor(() => expect(runs).toHaveBeenCalled());
    f.state.failure = "detail404"; f.sources[0]!.send("run-bad", "bad");
    await vi.waitFor(() => expect(health.mock.lastCall?.[0].failures).toHaveLength(1));
    f.state.failure = ""; f.state.details["run-bad"]!.activity.status = "queued"; f.sources[0]!.send("run-bad", "bad");
    await vi.waitFor(() => expect(runs.mock.lastCall?.[0].find((r: CodedActionRun) => r.id === "run-bad")?.status).toBe("queued"));
    expect(health.mock.lastCall?.[0].failures).toHaveLength(1);
    f.sources[0]!.open(); await vi.waitFor(() => expect(health.mock.lastCall?.[0].failures).toEqual([]));
  } finally { stops.forEach((stop) => stop()); }
});
it.each(["logout", "unsubscribe"])("%s blocks late callbacks and query results from an old generation", async (boundary) => {
  const f = activityFixture(); const runs = vi.fn(); const health = vi.fn();
  const stop = f.adapter.subscribeActions(runs); const off = f.adapter.subscribeActionReadHealth!(health);
  f.sources[0]!.open(); await vi.waitFor(() => expect(runs).toHaveBeenCalled());
  const oldSource = f.sources[0]!;
  const held = deferred<Response>(); let captured: Response | undefined;
  f.state.hold = async (path, response) => {
    if (path.endsWith("/bad/activities/run-bad")) { captured = response; return held.promise; }
    return response;
  };
  const pending = f.adapter.listActions(); await vi.waitFor(() => expect(captured !== undefined).toBe(true));
  f.state.hold = undefined;
  if (boundary === "logout") await f.adapter.logout(); else stop();
  const count = runs.mock.calls.length; const healthCount = health.mock.calls.length;
  held.resolve(captured!);
  await expect(pending).rejects.toBeInstanceOf(CancelledError);
  const requests = f.requests.length;
  oldSource.send("run-good", "good");
  expect(f.requests).toHaveLength(requests);
  expect(runs).toHaveBeenCalledTimes(count); expect(health).toHaveBeenCalledTimes(healthCount);
  if (boundary === "logout") expect(runs.mock.lastCall?.[0]).toEqual([]);
  stop(); off();
});

it("refreshes targeted hub detail while a remote source is unavailable and preserves host resource identity", async () => {
  const f = activityFixture();
  f.state.lists["0"] = [activity("0")]; f.state.details["run-0"] = detail("0");
  for (const run of Object.values(f.state.details)) { run.activity.resourceType = "project"; run.activity.resourceId = "same-project"; }
  f.state.failure = "page502";
  const runs = vi.fn(); const stop = f.adapter.subscribeActions(runs);
  try {
    f.sources[0]!.open(); await vi.waitFor(() => expect(runs.mock.lastCall?.[0]).toHaveLength(2));
    expect(runs.mock.lastCall?.[0].map((r: CodedActionRun) => r.stackId).sort()).toEqual(["0~same-project", "good~same-project"]);
    const before = f.requests.length;
    f.state.details["run-0"]!.activity.status = "queued"; f.sources[0]!.send("run-0", "0");
    await vi.waitFor(() => expect(runs.mock.lastCall?.[0].find((r: CodedActionRun) => r.id === "run-0")?.status).toBe("queued"));
    expect(f.requests.slice(before)).toHaveLength(1);
    expect(f.requests[before]).toContain("/environments/0/activities/run-0?");
  } finally { stop(); }
});
it("a capability identity rejection stays an authentication failure", async () => {
  const f = activityFixture();
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input) => String(input).includes("/capabilities") ? json(null, 401) : original(input));
  await expect(f.adapter.listActions()).rejects.toMatchObject({ outcome: { status: 401 } });
});
it("authentication rejection clears observations and blocks an older successful read", async () => {
  const f = activityFixture(); const runs = vi.fn(); const stop = f.adapter.subscribeActions(runs);
  try {
    f.sources[0]!.open(); await vi.waitFor(() => expect(runs).toHaveBeenCalled());
    const held = deferred<Response>(); let captured: Response | undefined;
    f.state.hold = async (path, response) => path.endsWith("/bad/activities/run-bad") ? (captured = response, held.promise) : response;
    const pending = f.adapter.listActions(); await vi.waitFor(() => expect(captured !== undefined).toBe(true));
    f.state.hold = undefined; f.state.failure = "page401"; f.sources[0]!.open();
    await vi.waitFor(() => expect(runs.mock.lastCall?.[0]).toEqual([]));
    held.resolve(captured!); await expect(pending).rejects.toBeInstanceOf(CancelledError);
    expect(runs.mock.lastCall?.[0]).toEqual([]);
  } finally { stop(); }
});
it.each(["logout", "unsubscribe"])("ignores a held targeted completion after %s", async (boundary) => {
  const f = activityFixture(); const runs = vi.fn(); const stop = f.adapter.subscribeActions(runs);
  let off = () => {};
  try {
    f.sources[0]!.open(); await vi.waitFor(() => expect(runs).toHaveBeenCalled());
    const held = deferred<Response>(); let captured: Response | undefined;
    f.state.details["run-good"]!.activity.status = "success";
    f.state.hold = async (path, response) => {
      if (path.endsWith("/good/activities/run-good")) { captured = response; return held.promise; }
      return response;
    };
    f.sources[0]!.send("run-good", "good");
    await vi.waitFor(() => expect(captured !== undefined).toBe(true));
    f.state.hold = undefined;
    if (boundary === "logout") await f.adapter.logout(); else stop();
    vi.useFakeTimers();
    held.resolve(captured!);
    await vi.runAllTimersAsync();
    vi.useRealTimers();
    const resumed = vi.fn(); off = f.adapter.subscribeActions(resumed);
    if (boundary === "logout") expect(resumed.mock.lastCall?.[0]).toEqual([]);
    else expect(resumed.mock.lastCall?.[0].find((r: CodedActionRun) => r.id === "run-good")?.status).toBe("running");
  } finally { vi.useRealTimers(); off(); stop(); }
});
it("rejects valid cross-host details claiming the same run UUID without hiding an unrelated healthy run", async () => {
  const f = activityFixture();
  f.state.lists.good = [activity("good", "shared-run"), activity("good")];
  f.state.lists.bad = [activity("bad", "shared-run")];
  f.state.hold = async (path, response) => {
    if (path.endsWith("/good/activities/shared-run")) return json(detail("good", "shared-run"));
    if (path.endsWith("/bad/activities/shared-run")) return json(detail("bad", "shared-run"));
    return response;
  };
  const health = vi.fn(); const stop = f.adapter.subscribeActionReadHealth!(health);
  try {
    expect((await f.adapter.listActions()).map((r) => [r.id, r.hostId])).toEqual([["run-good", "good"]]);
    expect(health.mock.lastCall?.[0].failures).toEqual([
      { hostId: "good", reason: "invalid-data" }, { hostId: "bad", reason: "invalid-data" },
    ]);
    f.state.lists.bad = [];
    expect((await f.adapter.listActions()).map((r) => r.id)).toEqual(["shared-run", "run-good"]);
    expect(health.mock.lastCall?.[0].failures).toEqual([]);
  } finally { stop(); }
});
