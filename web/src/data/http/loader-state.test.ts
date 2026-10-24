// Source: docklet-next/issues/211
// Revision: 00b0dacea1c004c60e92d1cf0a2025734a468188
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original shared-loader state matrix with controlled read and identity barriers.

import { CancelledError } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import { createActivityReader } from "./activity-reader";
import { activity, deferred, detail } from "./activity-reader.test-support";
import { createEventReader } from "./event-reader";

type Kind = "discovery" | "recovery" | "history";
function fixture(kind: Kind) {
  let generation = 0;
  const gates = Array.from({ length: 4 }, () => deferred<string>());
  const reached = Array.from({ length: 4 }, () => deferred<void>());
  const read = vi.fn(() => { const n = read.mock.calls.length - 1; reached[n]!.resolve(); return gates[n]!.promise; });
  const publish = vi.fn();
  const events = createEventReader(read, () => generation);
  const actions = createActivityReader({
    hosts: async () => ["good"], page: async () => ({ items: [activity("good")], incomplete: false }),
    detail: async () => { const status = await read(); const value = detail("good"); value.activity.status = status as typeof value.activity.status; return value; },
    publish, unauthorized: vi.fn(),
  });
  return {
    gates, reached, read, publish,
    load: () => {
      if (kind === "recovery" && read.mock.calls.length === 0) void actions.refresh(true).catch(() => undefined);
      return kind === "history" ? events.load() : actions.load().then((rows) => rows[0]!.status);
    },
    notify: () => { if (kind === "history") events.changed(); else void actions.refresh().catch(() => undefined); },
    replace: () => { generation++; actions.reset(); },
  };
}

for (const kind of ["discovery", "recovery", "history"] as const) {
  it(`${kind}: first-read burst queues exactly one fresh read and routes stale-marked callers`, async () => {
    const f = fixture(kind); const first = f.load(); await f.reached[0]!.promise;
    for (let i = 0; i < 30; i++) f.notify();
    const fresh = f.load(); expect(f.read).toHaveBeenCalledTimes(1);
    f.gates[0]!.resolve("running"); expect(await first).toBe("running");
    await f.reached[1]!.promise;
    f.gates[1]!.resolve("queued"); expect(await fresh).toBe("queued");
    expect(f.read).toHaveBeenCalledTimes(2);
  });
  it(`${kind}: notification after trailing start queues exactly one more fresh read`, async () => {
    const f = fixture(kind); const first = f.load(); await f.reached[0]!.promise;
    f.notify(); const second = f.load();
    f.gates[0]!.resolve("running"); await first; await f.reached[1]!.promise;
    for (let i = 0; i < 30; i++) f.notify();
    const third = f.load();
    f.gates[1]!.resolve("queued"); expect(await second).toBe("queued");
    await f.reached[2]!.promise;
    expect(f.read).toHaveBeenCalledTimes(3);
    f.gates[2]!.resolve("success"); expect(await third).toBe(kind === "history" ? "success" : "succeeded");
    expect(f.read).toHaveBeenCalledTimes(3);
  });
  it.each([false, true])(`${kind}: identity change cancels current and queued without publication (trailing=%s)`, async (trailing) => {
    const f = fixture(kind); let current = f.load(); await f.reached[0]!.promise; let index = 0;
    if (trailing) { f.notify(); const next = f.load(); f.gates[0]!.resolve("running"); await current; current = next; index = 1; await f.reached[1]!.promise; }
    f.notify(); const queued = f.load(); const old = current.catch((e: unknown) => e); const next = queued.catch((e: unknown) => e);
    f.replace(); const publications = f.publish.mock.calls.length;
    f.gates[index]!.resolve("private");
    expect(await old).toBeInstanceOf(CancelledError);
    expect(await next).toBeInstanceOf(CancelledError);
    expect(f.read).toHaveBeenCalledTimes(index + 1);
    expect(f.publish).toHaveBeenCalledTimes(publications);
  });
}
