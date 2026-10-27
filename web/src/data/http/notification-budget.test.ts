// Source: docklet-next/issues/211
// Revision: 79ee005c9dbf2fb261bd413a6d273c4cce71c2e3
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original deterministic progress, discovery and history notification budgets.

import { expect, it, vi } from "vitest";
import { activityFixture, deferred } from "./activity-reader.test-support";

it("fifty known-run messages share throttled targeted reads and one terminal flush", async () => {
  const f = activityFixture(); await f.adapter.listActions();
  const runs = vi.fn(); const changes = vi.fn();
  const stops = [f.adapter.subscribeActions(runs), f.adapter.subscribeChanges(changes)];
  const gate = deferred<void>(); const reached = deferred<void>();
  const before = f.requests.length; vi.useFakeTimers();
  f.state.hold = async (path, response) => {
    if (path.endsWith("/good/activities")) { reached.resolve(); await gate.promise; }
    return response;
  };
  try {
    f.sources[0]!.send("unknown", "good", "message"); await reached.promise;
    for (let i = 0; i < 50; i++) {
      f.sources[0]!.send("run-good", "good", "message");
      await vi.advanceTimersByTimeAsync(290);
    }
    const regular = f.requests.slice(before).filter((p) => /\/activities\/[^?]+/.test(p)).length;
    expect(regular).toBe(15);
    f.state.details["run-good"]!.activity.status = "success";
    f.sources[0]!.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: "2026-10-24T00:00:00Z", activity: { type: "message", environmentId: "good", activity: f.state.details["run-good"]!.activity } }), lastEventId: "" } as MessageEvent);
    await vi.advanceTimersByTimeAsync(0);
    expect(f.requests.slice(before).filter((p) => /\/activities\/[^?]+/.test(p))).toHaveLength(16);
    for (let i = 0; i < 10; i++) f.sources[0]!.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: "2026-10-24T00:00:00Z", activity: { type: "message", environmentId: "good", activity: f.state.details["run-good"]!.activity } }), lastEventId: "" } as MessageEvent);
    await vi.advanceTimersByTimeAsync(0);
    gate.resolve(); await vi.advanceTimersByTimeAsync(0);
    const requests = f.requests.slice(before);
    expect(requests.filter((p) => p.startsWith("/api/environments/good/activities?"))).toHaveLength(1);
    expect(requests.filter((p) => /\/activities\/[^?]+/.test(p))).toHaveLength(16);
    expect(runs.mock.lastCall?.[0].find((r: { id: string }) => r.id === "run-good")).toMatchObject({ status: "succeeded", output: [], log: [] });
    expect(changes.mock.calls.some(([topics]) => topics.includes("events"))).toBe(false);
  } finally { gate.resolve(); stops.forEach((stop) => stop()); vi.useRealTimers(); }
});

it.each(["password", "login"])("event-only observation after %s renewal does not invalidate event history, while an events message does", async (renewal) => {
  const f = activityFixture(); await f.adapter.getCurrentUser(); await f.adapter.listActions();
  const changes = vi.fn(); const stop = f.adapter.subscribeChanges(changes);
  try {
    if (renewal === "password") await f.adapter.changePassword("Example-password1!", "New-example-password2!");
    else await f.adapter.login("operator", "Example-password1!");
    const before = f.requests.length;
    const source = f.sources.at(-1)!;
    source.send("run-good", "good", "message");
    await Promise.resolve();
    expect(f.requests).toHaveLength(before);
    expect(changes.mock.calls.some(([topics]) => topics.includes("events"))).toBe(false);
    source.onmessage?.({ data: JSON.stringify({ channel: "events", timestamp: "2026-10-24T00:00:00Z", eventLog: { type: "changed", timestamp: "2026-10-24T00:00:00Z" } }), lastEventId: "" } as MessageEvent);
    expect(changes).toHaveBeenCalledExactlyOnceWith(["events"]);
  } finally { stop(); }
});
