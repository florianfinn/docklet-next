// Source: docklet-next/issues/211
// Revision: 00b0dacea1c004c60e92d1cf0a2025734a468188
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original adapter-level notification relevance, trailing freshness and logout guards.

import { expect, it, vi } from "vitest";
import { activity, activityFixture, deferred, detail } from "./activity-reader.test-support";

const discoveries = (requests: string[]) => requests.filter((p) => p.startsWith("/api/environments/good/activities?")).length;
const drain = () => vi.advanceTimersByTimeAsync(0);

it("unknown-run message discovers and publishes the new run", async () => {
  const f = activityFixture(); await f.adapter.listActions();
  const runs = vi.fn(); const stop = f.adapter.subscribeActions(runs);
  const before = discoveries(f.requests); vi.useFakeTimers();
  try {
    f.state.lists.good!.push(activity("good", "new-run")); f.state.details["new-run"] = detail("good", "new-run");
    f.sources[0]!.send("new-run", "good", "message"); await drain();
    expect(discoveries(f.requests) - before).toBe(1);
    expect(runs.mock.lastCall?.[0].map((r: { id: string }) => r.id)).toContain("new-run");
  } finally { stop(); vi.useRealTimers(); }
});

it.each(["updated", "message"])("terminal known-run %s uses exactly the required discovery path", async (type) => {
  const f = activityFixture(); await f.adapter.listActions();
  const runs = vi.fn(); const stop = f.adapter.subscribeActions(runs);
  const before = discoveries(f.requests); vi.useFakeTimers();
  try {
    f.state.details["run-good"]!.activity.status = "success"; f.state.lists.good = [];
    f.sources[0]!.send("run-good", "good", type); await drain();
    expect(discoveries(f.requests) - before).toBe(type === "updated" ? 1 : 0);
    const ids = runs.mock.lastCall?.[0].map((r: { id: string }) => r.id);
    expect(ids).toEqual(type === "updated" ? ["run-bad"] : ["run-good", "run-bad"]);
  } finally { stop(); vi.useRealTimers(); }
});

it("logout during held discovery drops queued work and prevents late publication", async () => {
  const f = activityFixture(); await f.adapter.getCurrentUser(); await f.adapter.listActions();
  const runs = vi.fn(); const stop = f.adapter.subscribeActions(runs);
  const held = deferred<void>(); const reached = deferred<void>(); vi.useFakeTimers();
  f.state.hold = async (path, response) => { if (path.endsWith("/good/activities")) { reached.resolve(); await held.promise; } return response; };
  try {
    f.sources[0]!.send("unknown-1", "good"); await reached.promise;
    f.sources[0]!.send("unknown-2", "good");
    await f.adapter.logout(); const reads = f.requests.length; const publications = runs.mock.calls.length;
    expect(runs.mock.lastCall?.[0]).toEqual([]);
    held.resolve(); await drain();
    expect(f.requests).toHaveLength(reads);
    expect(runs).toHaveBeenCalledTimes(publications);
    expect(runs.mock.lastCall?.[0]).toEqual([]);
  } finally { held.resolve(); stop(); vi.useRealTimers(); }
});

it.each(["discovery", "recovery"])("%s notification after trailing page discovers a new run in exactly one further read", async (kind) => {
  const f = activityFixture(); await f.adapter.listActions();
  const runs = vi.fn(); const stop = f.adapter.subscribeActions(runs);
  const first = deferred<void>(); const second = deferred<void>(); const reached = [deferred<void>(), deferred<void>()];
  let round = 0; const before = discoveries(f.requests); vi.useFakeTimers();
  f.state.hold = async (path, response) => {
    if (path.endsWith("/good/activities")) { round++; if (round === 1) { reached[0]!.resolve(); await first.promise; } }
    if (round === 2 && path.endsWith("/good/activities/run-good")) { reached[1]!.resolve(); await second.promise; }
    return response;
  };
  const notify = () => kind === "recovery" ? f.sources[0]!.open() : f.sources[0]!.send("unknown", "good");
  try {
    notify(); await reached[0]!.promise;
    for (let i = 0; i < 30; i++) notify();
    first.resolve(); await reached[1]!.promise;
    expect(discoveries(f.requests) - before).toBe(2);
    f.state.lists.good!.push(activity("good", "new-run")); f.state.details["new-run"] = detail("good", "new-run");
    for (let i = 0; i < 30; i++) notify();
    const fresh = f.adapter.listActions();
    second.resolve(); await drain();
    expect(discoveries(f.requests) - before).toBe(3);
    expect((await fresh).map((r) => r.id)).toContain("new-run");
    expect(runs.mock.lastCall?.[0].map((r: { id: string }) => r.id)).toContain("new-run");
  } finally { first.resolve(); second.resolve(); stop(); vi.useRealTimers(); }
});

it("held old discovery cannot start a read after logout and subsequent renewal", async () => {
  const f = activityFixture(); await f.adapter.getCurrentUser(); await f.adapter.listActions();
  const stop = f.adapter.subscribeActions(vi.fn());
  const held = deferred<Response>(); const reached = deferred<Response>(); vi.useFakeTimers();
  f.state.hold = async (path, response) => path.endsWith("/good/activities/run-good") ? (reached.resolve(response), held.promise) : response;
  try {
    const old = f.adapter.listActions(); const rejected = old.catch((e: unknown) => e);
    const response = await reached.promise; f.state.hold = undefined;
    f.sources[0]!.send("unknown", "good");
    await f.adapter.logout(); await f.adapter.login("operator", "Example-password1!");
    const requests = f.requests.length;
    held.resolve(response); await rejected; await drain();
    expect(f.requests).toHaveLength(requests);
  } finally { held.resolve(new Response()); stop(); vi.useRealTimers(); }
});
