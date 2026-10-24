// Source: docklet-next/issues/211
// Revision: 31a3237938f4b560d550c9f0a34ebc547fddde0d
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original demand, immediate identity cancellation and recovery latency regressions.

import { CancelledError, QueryClient, QueryObserver } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import { registerReadConsumer } from "../read-consumers";
import { queryKeys } from "../query-keys";
import { eventDemandFixture } from "./event-demand.test-support";
import { activityFixture, deferred } from "./activity-reader.test-support";
import { createActivityReader } from "./activity-reader";

const drain = () => vi.advanceTimersByTimeAsync(0);

it("history without a consumer ignores open, spaced messages and bursts; the next request is fresh", async () => {
  const f = eventDemandFixture(); const stop = f.adapter.subscribeChanges(vi.fn()); vi.useFakeTimers();
  try {
    f.source.open(); await drain(); expect(f.calls).toHaveLength(0);
    for (let i = 0; i < 5; i++) { f.notify(); await drain(); }
    for (let i = 0; i < 30; i++) f.notify(); await drain();
    expect(f.calls).toHaveLength(0);
    f.state.rows[0] = { ...f.state.rows[0]!, id: "fresh" };
    const rows = await f.adapter.listEvents();
    expect(rows).toHaveLength(792); expect(rows[0]?.id).toBe("fresh"); expect(f.calls).toHaveLength(8);
    for (let i = 0; i < 5; i++) { f.notify(); await drain(); }
    expect(f.calls).toHaveLength(8);
  } finally { stop(); vi.useRealTimers(); }
});

it("only a live history observer enables idle notifications; cached rows are not a consumer", async () => {
  const f = eventDemandFixture(); const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  const offDemand = registerReadConsumer(f.adapter.subscribeChanges, "events", () => client.getQueryCache().find({ queryKey: queryKeys.events })?.isActive() ?? false);
  const stop = f.adapter.subscribeChanges(vi.fn());
  const observer = new QueryObserver(client, { queryKey: queryKeys.events, queryFn: () => f.adapter.listEvents() });
  let offObserver = observer.subscribe(() => {}); vi.useFakeTimers();
  try {
    await drain(); expect(f.calls).toHaveLength(8);
    f.source.open(); await drain(); expect(f.calls).toHaveLength(16);
    f.notify(); await drain(); expect(f.calls).toHaveLength(24);
    offObserver(); offObserver = () => {};
    for (let i = 0; i < 5; i++) { f.notify(); await drain(); }
    expect(f.calls).toHaveLength(24);
    expect(client.getQueryData(queryKeys.events)).toHaveLength(792);
  } finally { offObserver(); stop(); offDemand(); client.clear(); vi.useRealTimers(); }
});

it("history generation wiring cancels old waiters before an abort-ignoring held page is released", async () => {
  const f = eventDemandFixture(); await f.adapter.getCurrentUser();
  const held = deferred<Response>(); const reached = deferred<Response>();
  f.state.hold = async (response, index) => { if (index === 0) { reached.resolve(response); return held.promise; } return response; };
  const old = f.adapter.listEvents(); let error: unknown;
  const settled = old.catch((reason: unknown) => { error = reason; }); const response = await reached.promise;
  vi.useFakeTimers();
  try {
    await f.adapter.logout(); await drain();
    expect(error).toBeInstanceOf(CancelledError);
    await f.adapter.login("operator", "Example-password1!");
    f.state.rows[0] = { ...f.state.rows[0]!, id: "new-identity" };
    const fresh = f.adapter.listEvents(); expect(fresh).not.toBe(old);
    const rows = await fresh; expect(rows[0]?.id).toBe("new-identity"); expect(f.calls).toHaveLength(9);
    held.resolve(response); await settled; await drain();
    expect(f.calls).toHaveLength(9);
  } finally { held.resolve(response); vi.useRealTimers(); }
});

it("reader reset rejects held current and queued waiters immediately and clears published runs", async () => {
  const held = deferred<string[]>(); const publish = vi.fn(); const hosts = vi.fn(() => held.promise);
  const reader = createActivityReader({ hosts, page: async () => ({ items: [], incomplete: false }), detail: vi.fn(), publish, unauthorized: vi.fn() });
  let oldError: unknown; let queuedError: unknown;
  const current = reader.load().catch((e: unknown) => { oldError = e; });
  const queued = reader.refresh().catch((e: unknown) => { queuedError = e; });
  reader.reset(); await Promise.resolve();
  try {
    expect(oldError).toBeInstanceOf(CancelledError); expect(queuedError).toBeInstanceOf(CancelledError);
    expect(publish.mock.lastCall?.[0]).toEqual([]); expect(hosts).toHaveBeenCalledTimes(1);
  } finally { held.resolve([]); await Promise.all([current, queued]); }
});

it("activity discovery without subscribers ignores recovery and unknown-run notifications", async () => {
  const f = activityFixture(); const stop = f.adapter.subscribeChanges(vi.fn()); vi.useFakeTimers();
  try {
    f.sources[0]!.open(); await drain();
    for (let i = 0; i < 30; i++) f.sources[0]!.send("unknown", "good");
    await drain(); expect(f.requests.filter((p) => /\/activities(?:\?|$)/.test(p))).toEqual([]);
    expect(await f.adapter.listActions()).toHaveLength(2);
  } finally { stop(); vi.useRealTimers(); }
});

it.each(["ordinary", "recovery"])("activity401 clears runs while an obsolete %s discovery response is held", async (kind) => {
  const f = activityFixture(); await f.adapter.getCurrentUser(); await f.adapter.listActions();
  const runs = vi.fn(); const stop = f.adapter.subscribeActions(runs);
  const held = deferred<Response>(); const reached = deferred<Response>(); vi.useFakeTimers();
  f.state.hold = async (path, response) => path.endsWith("/good/activities/run-good") ? (reached.resolve(response), held.promise) : response;
  if (kind === "recovery") f.sources[0]!.open();
  const old = f.adapter.listActions(); const rejected = old.catch((e: unknown) => e);
  const response = await reached.promise; f.state.hold = undefined;
  try {
    f.state.failure = "page401"; f.sources[0]!.open(); await drain();
    expect(runs.mock.lastCall?.[0]).toEqual([]);
    expect(await rejected).toMatchObject({ outcome: { status: 401 } });
    const publications = runs.mock.calls.length; const requests = f.requests.length;
    held.resolve(response); await drain();
    expect(runs).toHaveBeenCalledTimes(publications); expect(f.requests).toHaveLength(requests);
  } finally { held.resolve(response); stop(); vi.useRealTimers(); }
});
