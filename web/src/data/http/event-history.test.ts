// Source: docklet-next/issues/211
// Revision: 3cba55fd10c311e4a5b3361afabb8c93fca706a0
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original full-history request budget, sequential paging and coalescing tests.

import { expect, it, vi } from "vitest";
import { createHttpAdapter } from "./http-adapter";
import { ActivitySource, at, deferred, json } from "./activity-reader.test-support";

function fixture() {
  const state = {
    rows: Array.from({ length: 792 }, (_, id) => ({ id: `event-${id}`, type: "project.deploy", messageCode: "project.deploy", severity: "success", timestamp: at, createdAt: at, title: "Example" })),
    hold: undefined as undefined | ((response: Response, index: number) => Promise<Response>),
    status: 200,
  };
  const source = new ActivitySource();
  const calls: { start: number; limit: number }[] = [];
  let active = 0; let max = 0;
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname.endsWith("/capabilities")) return json([{ operationId: "listEvents", available: true, resourceRef: { kind: "hub", id: "0" } }]);
    if (url.pathname.endsWith("/events")) {
      const start = Number(url.searchParams.get("start")); const limit = Number(url.searchParams.get("limit"));
      calls.push({ start, limit }); active++; max = Math.max(max, active);
      // Reflect the producer's 100-row maximum even if a request asks for more.
      const response = json(state.rows.slice(start, start + Math.min(limit, 100)), state.status, state.rows.length);
      try { return state.hold ? await state.hold(response, calls.length - 1) : response; }
      finally { active--; }
    }
    throw new Error("Unexpected history fixture route");
  });
  return { state, source, calls, max: () => max, adapter: createHttpAdapter({ fetch: fetcher, eventSource: () => source as unknown as EventSource }) };
}

it("loads all 792 events in eight strictly sequential effective pages shared by concurrent consumers", async () => {
  const f = fixture();
  const reached = Array.from({ length: 8 }, () => deferred<void>());
  const gates = Array.from({ length: 8 }, () => deferred<Response>());
  f.state.hold = async (response, index) => { reached[index]!.resolve(); await gates[index]!.promise; return response; };
  const first = f.adapter.listEvents(); const second = f.adapter.listEvents();
  for (let i = 0; i < 8; i++) {
    await reached[i]!.promise;
    expect(f.calls).toHaveLength(i + 1);
    expect(f.calls[i]).toEqual({ start: i * 100, limit: 100 });
    gates[i]!.resolve(new Response());
  }
  const rows = await first;
  expect(rows).toHaveLength(792);
  expect(rows.map((row) => row.id)).toEqual(f.state.rows.map((row) => row.id));
  expect(await second).toEqual(rows);
  expect(f.calls).toHaveLength(8); expect(f.max()).toBe(1);
});

it("coalesces a burst of event invalidations into one trailing complete reload", async () => {
  const f = fixture(); const reached = deferred<void>(); const gate = deferred<void>();
  const stop = f.adapter.subscribeChanges(() => {});
  f.state.hold = async (response, index) => { if (index === 0) { reached.resolve(); await gate.promise; } return response; };
  try {
    const first = f.adapter.listEvents(); await reached.promise;
    f.state.rows[0] = { ...f.state.rows[0]!, id: "new-event" };
    const pending = [];
    for (let i = 0; i < 20; i++) {
      f.source.onmessage?.({ data: JSON.stringify({ channel: "events", timestamp: at, eventLog: { type: "changed", timestamp: at } }), lastEventId: "" } as MessageEvent);
      pending.push(f.adapter.listEvents());
    }
    expect(f.calls).toHaveLength(1);
    gate.resolve();
    expect((await first)[0]?.id).toBe("event-0");
    for (const result of await Promise.all(pending)) {
      expect(result).toHaveLength(792); expect(result[0]?.id).toBe("new-event");
    }
    expect(f.calls).toHaveLength(16); expect(f.max()).toBe(1);
  } finally { gate.resolve(); stop(); }
});

it("does not cache a failed full history as success or prevent a deliberate retry", async () => {
  const f = fixture(); f.state.status = 500;
  await expect(f.adapter.listEvents()).rejects.toMatchObject({ outcome: { status: 500 } });
  expect(f.calls).toHaveLength(1);
  f.state.status = 200;
  expect(await f.adapter.listEvents()).toHaveLength(792);
  expect(f.calls).toHaveLength(9);
});

it("event arriving during trailing history queues one more sequential 792-row observation", async () => {
  const f = fixture(); const firstGate = deferred<void>(); const secondGate = deferred<void>();
  const firstReached = deferred<void>(); const secondReached = deferred<void>();
  const stop = f.adapter.subscribeChanges(() => {});
  f.state.hold = async (response, index) => {
    if (index === 0) { firstReached.resolve(); await firstGate.promise; }
    if (index === 8) { secondReached.resolve(); await secondGate.promise; }
    return response;
  };
  const notify = () => f.source.onmessage?.({ data: JSON.stringify({ channel: "events", timestamp: at, eventLog: { type: "changed", timestamp: at } }), lastEventId: "" } as MessageEvent);
  try {
    const first = f.adapter.listEvents(); await firstReached.promise;
    f.state.rows[0] = { ...f.state.rows[0]!, id: "second" }; notify();
    const second = f.adapter.listEvents();
    firstGate.resolve(); expect((await first)[0]?.id).toBe("event-0"); await secondReached.promise;
    f.state.rows[0] = { ...f.state.rows[0]!, id: "third" };
    for (let i = 0; i < 30; i++) notify();
    const third = f.adapter.listEvents(); secondGate.resolve();
    expect((await second)[0]?.id).toBe("second");
    const rows = await third;
    expect(rows).toHaveLength(792); expect(rows[0]?.id).toBe("third");
    expect(f.calls).toHaveLength(24); expect(f.max()).toBe(1);
  } finally { firstGate.resolve(); secondGate.resolve(); stop(); }
});
