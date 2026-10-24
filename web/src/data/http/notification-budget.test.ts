// Source: docklet-next/issues/211
// Revision: 79ee005c9dbf2fb261bd413a6d273c4cce71c2e3
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original deterministic progress, discovery and history notification budgets.

import { expect, it, vi } from "vitest";
import { activityFixture, deferred } from "./activity-reader.test-support";

it("ten known-run progress messages remain targeted during one discovery and do not invalidate events", async () => {
  const f = activityFixture(); await f.adapter.listActions();
  const runs = vi.fn(); const changes = vi.fn();
  const stops = [f.adapter.subscribeActions(runs), f.adapter.subscribeChanges(changes)];
  const gate = deferred<void>(); const reached = deferred<void>();
  const targets = Array.from({ length: 10 }, () => deferred<void>()); let targeted = 0;
  const before = f.requests.length;
  vi.useFakeTimers();
  f.state.hold = async (path, response) => {
    if (path.endsWith("/good/activities")) { reached.resolve(); await gate.promise; }
    if (path.endsWith("/good/activities/run-good") && targeted < 10) targets[targeted++]!.resolve();
    return response;
  };
  try {
    f.sources[0]!.send("unknown", "good", "message");
    const current = f.adapter.listActions(); await reached.promise;
    for (let i = 0; i < 10; i++) {
      f.sources[0]!.send("run-good", "good", "message");
      await targets[i]!.promise;
    }
    gate.resolve(); await current;
    await vi.advanceTimersByTimeAsync(0);
    const requests = f.requests.slice(before);
    expect(requests.filter((p) => p.startsWith("/api/environments/good/activities?"))).toHaveLength(1);
    expect(requests.filter((p) => /\/activities\/[^?]+/.test(p))).toHaveLength(12);
    expect(changes.mock.calls.some(([topics]) => topics.includes("events"))).toBe(false);
  } finally { gate.resolve(); stops.forEach((stop) => stop()); vi.useRealTimers(); }
});

it.each(["password", "login"])("activity after %s renewal does not invalidate event history, while an events message does", async (renewal) => {
  const f = activityFixture(); await f.adapter.getCurrentUser(); await f.adapter.listActions();
  const changes = vi.fn(); const stop = f.adapter.subscribeChanges(changes);
  try {
    if (renewal === "password") await f.adapter.changePassword("Example-password1!", "New-example-password2!");
    else await f.adapter.login("operator", "Example-password1!");
    const before = f.requests.length;
    const source = f.sources.at(-1)!;
    source.send("run-good", "good", "message");
    await vi.waitFor(() => expect(f.requests.length).toBeGreaterThan(before));
    expect(changes.mock.calls.some(([topics]) => topics.includes("events"))).toBe(false);
    source.onmessage?.({ data: JSON.stringify({ channel: "events", timestamp: "2026-10-24T00:00:00Z", eventLog: { type: "changed", timestamp: "2026-10-24T00:00:00Z" } }), lastEventId: "" } as MessageEvent);
    expect(changes).toHaveBeenCalledExactlyOnceWith(["events"]);
  } finally { stop(); }
});
