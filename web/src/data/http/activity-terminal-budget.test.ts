// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Terminal deduplication across status-less messages and reordered reads (#225).

import { expect, it, vi } from "vitest";
import { activityFixture, at, deferred } from "./activity-reader.test-support";

const targeted = (requests: string[]) => requests.filter((path) => path.includes("/good/activities/run-good?")).length;
function terminal(f: ReturnType<typeof activityFixture>) {
  f.state.details["run-good"]!.activity.status = "success";
  f.sources[0]!.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: at, activity: { type: "message", environmentId: "good", activity: f.state.details["run-good"]!.activity } }), lastEventId: "" } as MessageEvent);
}

it("status-less messages cannot re-arm the bypass for a duplicate terminal transition", async () => {
  const f = activityFixture(); await f.adapter.listActions(); const off = f.adapter.subscribeActions(vi.fn()); vi.useFakeTimers();
  try {
    terminal(f); await vi.advanceTimersByTimeAsync(0);
    for (let i = 0; i < 20; i++) { f.sources[0]!.send("run-good", "good", "message"); terminal(f); await vi.advanceTimersByTimeAsync(0); }
    expect(targeted(f.requests)).toBe(1);
    await vi.advanceTimersByTimeAsync(1_000); expect(targeted(f.requests)).toBe(2);
  } finally { off(); vi.useRealTimers(); }
});

it("a newly observed active execution permits its own terminal flush", async () => {
  const f = activityFixture(); await f.adapter.listActions(); const off = f.adapter.subscribeActions(vi.fn()); vi.useFakeTimers();
  try {
    terminal(f); await vi.advanceTimersByTimeAsync(1_000);
    f.state.details["run-good"]!.activity.status = "running"; f.sources[0]!.send("run-good", "good", "message"); await vi.advanceTimersByTimeAsync(0);
    expect(targeted(f.requests)).toBe(2);
    terminal(f); await vi.advanceTimersByTimeAsync(0); expect(targeted(f.requests)).toBe(3);
  } finally { off(); vi.useRealTimers(); }
});

it("an active response started before a terminal notification cannot reset its deduplication", async () => {
  const f = activityFixture(); await f.adapter.listActions(); const off = f.adapter.subscribeActions(vi.fn());
  const held = deferred<Response>(), reached = deferred<Response>(); let first = true;
  f.state.hold = async (path, response) => { if (first && path.endsWith("/good/activities/run-good")) { first = false; reached.resolve(response); return held.promise; } return response; };
  vi.useFakeTimers();
  try {
    f.sources[0]!.send("run-good", "good", "message"); const old = await reached.promise;
    terminal(f); held.resolve(old); await vi.advanceTimersByTimeAsync(0);
    expect(targeted(f.requests)).toBe(2);
    f.sources[0]!.send("run-good", "good", "message"); terminal(f); await vi.advanceTimersByTimeAsync(0);
    expect(targeted(f.requests)).toBe(2);
  } finally { held.resolve(new Response("{}")); off(); vi.useRealTimers(); }
});
