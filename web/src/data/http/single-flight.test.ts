// Source: docklet-next/issues/211
// Revision: 00b0dacea1c004c60e92d1cf0a2025734a468188
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original controlled state-table coverage, waiter routing and generation cancellation.

import { CancelledError } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import { deferred } from "./activity-reader.test-support";
import { createSingleFlight } from "./single-flight";

function fixture() {
  let generation = 0;
  const gates = Array.from({ length: 4 }, () => deferred<string>());
  const read = vi.fn(() => gates[read.mock.calls.length - 1]!.promise);
  const publish = vi.fn();
  const flight = createSingleFlight(read, () => generation, publish);
  return { flight, gates, read, publish, replace: () => { generation++; } };
}

it.each(["request", "notify"] as const)("idle %s starts one read; running clean requests join; completion returns idle", async (method) => {
  const f = fixture(); const first = f.flight[method]();
  expect(f.flight.request()).toBe(first);
  expect(f.read).toHaveBeenCalledTimes(1);
  f.gates[0]!.resolve("one"); expect(await first).toBe("one");
  expect(f.publish).toHaveBeenCalledExactlyOnceWith("one");
  const second = f.flight.request(); expect(second).not.toBe(first);
  expect(f.read).toHaveBeenCalledTimes(2);
  f.gates[1]!.resolve("two"); await second;
});

it("dirty requests wait for next while earlier callers receive current; bursts queue exactly one", async () => {
  const f = fixture(); const first = f.flight.request();
  const next = f.flight.notify();
  for (let i = 0; i < 30; i++) expect(f.flight.notify()).toBe(next);
  expect(f.flight.request()).toBe(next);
  expect(next).not.toBe(first);
  expect(f.read).toHaveBeenCalledTimes(1);
  f.gates[0]!.resolve("old"); expect(await first).toBe("old");
  expect(f.read).toHaveBeenCalledTimes(2);
  expect(f.flight.request()).toBe(next);
  f.gates[1]!.resolve("fresh"); expect(await next).toBe("fresh");
  expect(f.read).toHaveBeenCalledTimes(2);
  expect(f.publish.mock.calls).toEqual([["old"], ["fresh"]]);
});

it("notification during a trailing read re-arms exactly one pending slot", async () => {
  const f = fixture(); const first = f.flight.request(); const second = f.flight.notify();
  f.gates[0]!.resolve("one"); await first;
  expect(f.read).toHaveBeenCalledTimes(2);
  const third = f.flight.notify();
  expect(third).not.toBe(second);
  for (let i = 0; i < 30; i++) expect(f.flight.notify()).toBe(third);
  expect(f.flight.request()).toBe(third);
  f.gates[1]!.resolve("two"); expect(await second).toBe("two");
  expect(f.read).toHaveBeenCalledTimes(3);
  f.gates[2]!.resolve("three"); expect(await third).toBe("three");
  expect(f.publish.mock.calls).toEqual([["one"], ["two"], ["three"]]);
});

it.each([false, true])("generation change during read cancels both waiters and never starts trailing (trailing=%s)", async (trailing) => {
  const f = fixture(); let current = f.flight.request(); let index = 0;
  if (trailing) { const next = f.flight.notify(); f.gates[0]!.resolve("one"); await current; current = next; index = 1; }
  const queued = f.flight.notify();
  const old = current.catch((e: unknown) => e); const next = queued.catch((e: unknown) => e);
  const publications = f.publish.mock.calls.length;
  f.replace(); f.gates[index]!.resolve("private");
  expect(await old).toBeInstanceOf(CancelledError);
  expect(f.read).toHaveBeenCalledTimes(index + 1);
  expect(await next).toBeInstanceOf(CancelledError);
  expect(f.publish).toHaveBeenCalledTimes(publications);
});

it("replacement starts distinct work; explicit cancellation rejects immediately without late publication", async () => {
  const f = fixture(); const old = f.flight.request(); const queued = f.flight.notify();
  f.replace(); const fresh = f.flight.request();
  expect(fresh).not.toBe(old); expect(f.read).toHaveBeenCalledTimes(2);
  await expect(old).rejects.toBeInstanceOf(CancelledError);
  await expect(queued).rejects.toBeInstanceOf(CancelledError);
  f.gates[1]!.resolve("fresh"); expect(await fresh).toBe("fresh");
  const third = f.flight.request(); f.flight.cancel();
  await expect(third).rejects.toBeInstanceOf(CancelledError);
  f.gates[0]!.resolve("old"); f.gates[2]!.resolve("cancelled");
  await Promise.resolve();
  expect(f.publish.mock.calls).toEqual([["fresh"]]);
});

it("a failed read rejects shared callers and permits a queued retry or later request", async () => {
  const f = fixture(); const first = f.flight.request(); const next = f.flight.notify();
  f.gates[0]!.reject(new Error("offline")); await expect(first).rejects.toThrow("offline");
  expect(f.read).toHaveBeenCalledTimes(2);
  f.gates[1]!.resolve("recovered"); expect(await next).toBe("recovered");
  const retry = f.flight.request(); f.gates[2]!.resolve("later"); expect(await retry).toBe("later");
});
