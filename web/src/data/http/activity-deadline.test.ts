// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original list-only discovery, bounded reads and demand regression coverage (#225, #223).

import { CancelledError } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import { activityDeadline, activityTimeout } from "./activity-deadline";
import { createSingleFlight } from "./single-flight";
import { createActivityReader } from "./activity-reader";
import { activity, detail, deferred } from "./activity-reader.test-support";

it("aborts a held response body at 40 seconds with a typed timeout rather than cancellation", async () => {
  vi.useFakeTimers(); const reached = deferred<void>(); let signal: AbortSignal | undefined;
  try {
    const pending = activityDeadline(async (owned) => {
      signal = owned;
      return new Promise<string>((_resolve, reject) => { owned.addEventListener("abort", () => reject(owned.reason), { once: true }); reached.resolve(); });
    });
    const rejected = pending.catch((error: unknown) => error); await reached.promise;
    await vi.advanceTimersByTimeAsync(39_999); expect(signal!.aborted).toBe(false);
    await vi.advanceTimersByTimeAsync(1); expect(signal!.aborted).toBe(true);
    expect(await rejected).toMatchObject({ outcome: { code: "activity.read-timeout" } });
  } finally { vi.useRealTimers(); }
});

it.each([false, true])("timeout starts only a genuine pending signal once (pending=%s)", async (pendingSignal) => {
  vi.useFakeTimers(); const gates = [deferred<string>(), deferred<string>()]; const signals: AbortSignal[] = [];
  const read = vi.fn((signal: AbortSignal) => { signals.push(signal); return gates[signals.length - 1]!.promise; });
  const publish = vi.fn(); const flight = createSingleFlight(read, () => 0, { deadlineMs: 40_000, timeout: activityTimeout, hasConsumer: () => true, publish });
  try {
    const first = flight.request(); const rejected = first.catch((error: unknown) => error);
    const next = pendingSignal ? flight.notify() : undefined;
    await vi.advanceTimersByTimeAsync(40_000);
    expect(await rejected).toMatchObject({ outcome: { code: "activity.read-timeout" } });
    expect(signals[0]!.aborted).toBe(true); expect(read).toHaveBeenCalledTimes(pendingSignal ? 2 : 1);
    gates[0]!.resolve("obsolete"); await vi.advanceTimersByTimeAsync(0); expect(publish).not.toHaveBeenCalled();
    if (pendingSignal) { gates[1]!.resolve("fresh"); expect(await next).toBe("fresh"); }
    await vi.advanceTimersByTimeAsync(80_000); expect(read).toHaveBeenCalledTimes(pendingSignal ? 2 : 1);
  } finally { flight.cancel(); vi.useRealTimers(); }
});

it("the protected replacement shares its original 80-second lineage bound", async () => {
  vi.useFakeTimers(); const read = vi.fn(() => new Promise<string>(() => {}));
  const flight = createSingleFlight(read, () => 0, { deadlineMs: 70_000, lineageMs: 80_000, timeout: activityTimeout, hasConsumer: () => true });
  try {
    const first = flight.request(); const rejected = first.catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(60_000); flight.markError(); const replacement = flight.recover(); const secondError = replacement.catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(19_999); expect(read).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(1); expect(await rejected).toMatchObject({ outcome: { code: "activity.read-timeout" } });
    expect(await secondError).toMatchObject({ outcome: { code: "activity.read-timeout" } });
  } finally { flight.cancel(); vi.useRealTimers(); }
});

it.each([false, true])("last summary consumer drops only notification-only pending work (explicit=%s)", async (explicit) => {
  let consumer = true; const gates = [deferred<string>(), deferred<string>()]; const read = vi.fn(() => gates[read.mock.calls.length - 1]!.promise);
  const flight = createSingleFlight(read, () => 0, { hasConsumer: () => consumer });
  const first = flight.request(); const queued = flight.notify(); if (explicit) expect(flight.request()).toBe(queued);
  consumer = false; gates[0]!.resolve("old"); expect(await first).toBe("old");
  expect(read).toHaveBeenCalledTimes(explicit ? 2 : 1);
  if (explicit) gates[1]!.resolve("fresh"); expect(await queued).toBe(explicit ? "fresh" : undefined);
});

it("a read started after the stream error is useful and cannot be interrupted by open", async () => {
  const gate = deferred<string>(); let signal!: AbortSignal;
  const read = vi.fn((owned: AbortSignal) => { signal = owned; return gate.promise; });
  const flight = createSingleFlight(read, () => 0, { hasConsumer: () => true });
  flight.markError(); const first = flight.request(); const pending = flight.recover();
  expect(signal.aborted).toBe(false); expect(read).toHaveBeenCalledTimes(1);
  flight.cancel(); await expect(first).rejects.toBeInstanceOf(CancelledError); await expect(pending).rejects.toBeInstanceOf(CancelledError);
});

it("an explicit pending aggregate waiter expires 90 seconds after joining, without draining future work", async () => {
  vi.useFakeTimers(); const membership = [deferred<string[]>(), deferred<string[]>()];
  const list = deferred<void>(); const started = deferred<void>();
  const hosts = vi.fn(() => membership[hosts.mock.calls.length - 1]!.promise);
  const reader = createActivityReader({ hosts, page: async () => { started.resolve(); await list.promise; return { items: [activity("good")], incomplete: false }; },
    detail: async () => detail("good"), publish: vi.fn(), unauthorized: vi.fn() });
  try {
    const first = reader.load(); await vi.advanceTimersByTimeAsync(10_000);
    void reader.refresh().catch(() => undefined); const later = reader.load(); const failed = later.catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(29_000); membership[0]!.resolve(["good"]); await started.promise;
    await vi.advanceTimersByTimeAsync(39_000); list.resolve(); expect(await first).toHaveLength(1);
    expect(hosts).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(21_999); let settled = false; void later.catch(() => { settled = true; });
    await vi.advanceTimersByTimeAsync(0); expect(settled).toBe(false);
    await vi.advanceTimersByTimeAsync(1); expect(settled).toBe(true); expect(await failed).toMatchObject({ outcome: { code: "activity.read-timeout" } });
    const retry = reader.load(); expect(retry).not.toBe(later); membership[1]!.resolve(["good"]); expect(await retry).toHaveLength(1);
  } finally { reader.invalidate(); vi.useRealTimers(); }
});
