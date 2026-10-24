// Source: docklet-next/issues/211
// Revision: 79ee005c9dbf2fb261bd413a6d273c4cce71c2e3
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original deterministic progress, discovery and history notification budgets.

import { expect, it, vi } from "vitest";
import { activityFixture, deferred } from "./activity-reader.test-support";
import { createActivityReader } from "./activity-reader";
import { activity, detail } from "./activity-reader.test-support";
import { createEventReader } from "./event-reader";

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

it("unknown activity bursts queue only one trailing discovery and current callers settle before it", async () => {
  const first = deferred<void>(); const second = deferred<void>(); const reached = deferred<void>(); let count = 0;
  const reads = { hosts: vi.fn(async () => ["good"]), page: vi.fn(async () => {
    const n = ++count;
    if (n === 1) await first.promise;
    if (n === 2) { reached.resolve(); await second.promise; }
    return { items: [activity("good")], incomplete: false };
  }), detail: vi.fn(async () => detail("good")), publish: vi.fn(), unauthorized: vi.fn() };
  const reader = createActivityReader(reads);
  const current = reader.load();
  const trailing = reader.refresh();
  for (let i = 0; i < 10; i++) expect(reader.refresh()).toBe(trailing);
  first.resolve();
  expect(await current).toHaveLength(1);
  await reached.promise;
  const following = reader.load(); expect(following).not.toBe(current);
  for (let i = 0; i < 10; i++) expect(reader.refresh()).toBe(following);
  second.resolve(); await following;
  expect(reads.hosts).toHaveBeenCalledTimes(2);
});

it("event notifications during the trailing history cannot queue a third history", async () => {
  const gates = [deferred<string[]>(), deferred<string[]>()];
  const read = vi.fn().mockReturnValueOnce(gates[0]!.promise).mockReturnValueOnce(gates[1]!.promise).mockResolvedValue(["unwanted"]);
  const reader = createEventReader<string[]>(read, () => 0);
  const current = reader.load(); reader.changed(); gates[0]!.resolve(["old"]);
  await vi.waitFor(() => expect(read).toHaveBeenCalledTimes(2));
  for (let i = 0; i < 10; i++) reader.changed();
  gates[1]!.resolve(["fresh"]);
  expect(await current).toEqual(["fresh"]);
  expect(read).toHaveBeenCalledTimes(2);
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
