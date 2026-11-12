// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proofs for one cold Dashboard events read per connection through the real HTTP adapter (L4-e).

import { afterEach, expect, it, vi } from "vitest";

import { registerReadConsumer } from "../read-consumers";
import { at } from "./activity-reader.test-support";
import { INITIAL_FRAME_WAIT_MS } from "./dashboard-events";
import { response } from "./runtime-commands.test-support";
import { sessionFixture } from "./session-lifecycle.test-support";

afterEach(() => { vi.useRealTimers(); });
const flush = async () => { for (let i = 0; i < 40; i++) await Promise.resolve(); };
const row = (id: string) => ({ id, type: "project.deploy", messageCode: "project.deploy", severity: "success", timestamp: at, createdAt: at, title: "Example" });

/** The hub's bounded window read; every read reports what it returned. */
async function hub() {
  const rows = [row("e1")];
  const reads: Array<{ since: string | null; limit: string | null; ids: string[] }> = [];
  const f = sessionFixture((url) => {
    if (url.pathname !== "/api/events" || !url.searchParams.has("since")) return undefined;
    reads.push({ since: url.searchParams.get("since"), limit: url.searchParams.get("limit"), ids: rows.map((r) => r.id) });
    return response({ success: true, data: [...rows], pagination: { totalItems: rows.length } });
  });
  await f.adapter.getCurrentUser();
  // TanStack's active Dashboard query is the read consumer; the listener stands for its invalidation.
  const offConsumer = registerReadConsumer(f.adapter.subscribeChanges, "dashboardEvents", () => true);
  const invalidations: number[] = [];
  const refetched: Array<Promise<{ events: Array<{ id: string }> }>> = [];
  // Like the active query, an invalidation reads at once (and joins a refresh already running).
  const stop = f.adapter.subscribeChanges((topics) => {
    if (!topics.includes("dashboardEvents")) return;
    invalidations.push(Date.now());
    refetched.push(f.adapter.listDashboardEvents());
  });
  const frame = (index = 0) => f.sources[index]!.onmessage?.({ data: JSON.stringify({ channel: "events", timestamp: at, eventLog: { type: "changed", timestamp: at } }), lastEventId: "" } as MessageEvent);
  return { ...f, rows, reads, invalidations, refetched, frame, stop: () => { stop(); offConsumer(); } };
}

it("L4-e a cold load waits for the initial frame within 1 s, reads once and absorbs the frame and the stream open", async () => {
  vi.useFakeTimers();
  const h = await hub();
  const load = h.adapter.listDashboardEvents();
  await flush();
  expect(h.reads.length).toBe(0);
  h.sources[0]!.open();
  await vi.advanceTimersByTimeAsync(200);
  h.frame();
  const page = await load;
  await vi.advanceTimersByTimeAsync(5_000);
  expect([h.reads.length, h.invalidations.length]).toEqual([1, 0]);
  expect([page.totalItems, page.events.map((e) => e.id), h.reads[0]!.limit]).toEqual([1, ["e1"], "500"]);
  expect(Math.abs(Date.parse(h.reads[0]!.since!) - (Date.now() - 24 * 3_600_000 - 5_200)) < 1_000).toBe(true);
  h.stop();
});

it("L4-e an initial frame later than 1 s: the fallback read stands and the late frame refreshes once", async () => {
  vi.useFakeTimers();
  const h = await hub();
  const load = h.adapter.listDashboardEvents();
  await vi.advanceTimersByTimeAsync(INITIAL_FRAME_WAIT_MS - 10);
  expect(h.reads.length).toBe(0);
  await vi.advanceTimersByTimeAsync(20);
  expect(h.reads.length).toBe(1);
  expect((await load).events.map((e) => e.id)).toEqual(["e1"]);
  // A commit lands after the fallback snapshot and before the producer subscribed; only its frame signals it.
  h.rows.unshift(row("e2"));
  h.sources[0]!.open();
  h.frame();
  await flush();
  expect(h.invalidations.length).toBe(1);
  const refreshed = await h.refetched[0]!;
  expect([h.reads.length, refreshed.events.map((e) => e.id)]).toEqual([2, ["e2", "e1"]]);
  h.stop();
});

it("L4-e later frames refresh once each burst; a reconnect's initial frame refreshes; an identity change starts over", async () => {
  vi.useFakeTimers();
  const h = await hub();
  const load = h.adapter.listDashboardEvents();
  h.sources[0]!.open(); h.frame();
  await load;
  h.frame(); h.frame(); h.frame();
  await Promise.all(h.refetched);
  // One running refresh plus one coalesced follow-up for the burst.
  expect([h.reads.length, h.invalidations.length]).toEqual([3, 3]);
  const before = h.reads.length;
  h.sources[0]!.failClosed();
  await vi.advanceTimersByTimeAsync(10_000);
  expect(h.sources.length).toBe(2);
  h.sources[1]!.open();
  await flush();
  expect(h.reads.length).toBe(before);
  h.frame(1);
  await Promise.all(h.refetched);
  expect(h.reads.length).toBe(before + 1);
  // An identity change starts over: the next connection's initial frame is absorbed by a read waiting for it.
  await h.adapter.logout();
  await h.adapter.login("operator", "example-password");
  const next = h.adapter.listDashboardEvents();
  await flush();
  const source = h.sources.at(-1)!;
  source.open();
  source.onmessage?.({ data: JSON.stringify({ channel: "events", timestamp: at, eventLog: { type: "changed", timestamp: at } }), lastEventId: "" } as MessageEvent);
  await next;
  await Promise.all(h.refetched);
  expect(h.reads.length).toBe(before + 2);
  h.stop();
});
