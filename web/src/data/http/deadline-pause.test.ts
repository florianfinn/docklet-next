// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proofs that request timers spend only active time across a hub maintenance hold.

import { afterEach, expect, it, vi } from "vitest";
import { deferred } from "./activity-reader.test-support";
import { pausableTimeout, setDeadlinePause } from "./deadline-pause";
import { createHostReadAdmission } from "./host-read-admission";
import { createSingleFlight } from "./single-flight";

const hold = {};
const flush = async () => { for (let i = 0; i < 20; i++) await Promise.resolve(); };
afterEach(() => { setDeadlinePause(hold, false); vi.useRealTimers(); });

it("A-helper a request timer parks during a hold and fires after its remaining active time", async () => {
  vi.useFakeTimers();
  const fire = vi.fn();
  pausableTimeout(10_000, fire);
  await vi.advanceTimersByTimeAsync(4_000);
  setDeadlinePause(hold, true);
  await vi.advanceTimersByTimeAsync(60_000);
  setDeadlinePause(hold, false);
  await vi.advanceTimersByTimeAsync(5_999);
  const early = fire.mock.calls.length;
  await vi.advanceTimersByTimeAsync(1);
  expect([early, fire.mock.calls.length]).toEqual([0, 1]);
});

it("A3 a single-flight deadline does not expire while its read waits in a hold", async () => {
  vi.useFakeTimers();
  const gate = deferred<string>();
  const flight = createSingleFlight(() => gate.promise, () => 0, { deadlineMs: 10_000, lineageMs: 20_000 });
  const read = flight.request();
  setDeadlinePause(hold, true);
  await vi.advanceTimersByTimeAsync(45_000);
  setDeadlinePause(hold, false);
  await vi.advanceTimersByTimeAsync(2_000);
  gate.resolve("live");
  await expect(read).resolves.toBe("live");
});

it("A17 an abort after a long hold but short active time does not mark the host held", async () => {
  vi.useFakeTimers();
  const gate = deferred<Response>(), controller = new AbortController();
  const path = (id: number) => `/api/environments/direct/containers/${id}`;
  const physical = vi.fn(async (input: RequestInfo | URL) => String(input) === path(1) ? gate.promise : Response.json({ ok: true }));
  const fetch = createHostReadAdmission(physical);
  const first = fetch(path(1), { signal: controller.signal });
  const held = await Promise.all([2, 3, 4, 5].map((i) => fetch(path(i))));
  const next = fetch(path(6));
  setDeadlinePause(hold, true);
  await vi.advanceTimersByTimeAsync(30_000);
  setDeadlinePause(hold, false);
  await vi.advanceTimersByTimeAsync(1_000);
  controller.abort(); await flush();
  expect(physical.mock.calls.length).toBe(6);
  gate.resolve(Response.json({ late: true }));
  await first.then((r) => r.text(), () => undefined); await (await next).text(); await Promise.all(held.map((r) => r.text()));
});
