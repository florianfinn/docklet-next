// Source: docklet-next/issues/360
// Revision: 4e71e45a2297a4f2719188cdf15c3e4d586b5357
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original real HTTP adapter fixtures for shell reads and terminal ownership. M20 #360: shell list reads and target-specific terminal invalidation.

import { CancelledError } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { hostReadDeadline, inventoryReadBudget } from "./host-read-deadline";
import { hardeningFixture } from "./web-hardening.test-support";
import { deferred } from "./activity-reader.test-support";

afterEach(() => vi.useRealTimers());
it("parent abort settles a read and ignores a late successful response", async () => {
  const parent = new AbortController(), gate = deferred<string>(); let signal!: AbortSignal;
  const pending = hostReadDeadline(30_000, "http.error")((owned) => { signal = owned; return gate.promise; }, parent.signal);
  const failed = pending.catch((error: unknown) => error); parent.abort();
  expect(await failed).toBeInstanceOf(CancelledError); expect(signal.aborted).toBe(true);
  gate.resolve("stale"); expect(await failed).toBeInstanceOf(CancelledError);
});
it("an already cancelled query never starts a physical request", async () => {
  const parent = new AbortController(); parent.abort(); const read = vi.fn();
  await expect(hostReadDeadline(30_000, "http.error")(read, parent.signal)).rejects.toBeInstanceOf(CancelledError);
  expect(read).not.toHaveBeenCalled();
});
it("one budget includes prerequisite queuing and all pagination phases", async () => {
  vi.useFakeTimers(); const budget = inventoryReadBudget();
  await budget(async () => { await vi.advanceTimersByTimeAsync(20_000); });
  const held = budget(() => new Promise<void>(() => {})); const failed = held.catch((error: unknown) => error);
  await vi.advanceTimersByTimeAsync(9_999); let settled = false; void held.catch(() => { settled = true; }); expect(settled).toBe(false);
  await vi.advanceTimersByTimeAsync(1); expect(await failed).toMatchObject({ outcome: { code: "http.error" } });
});
it.each(["listStacks", "listContainers", "listImages", "listVolumes", "listNetworkSummaries"] as const)("%s passes cancellation to the real HTTP transport", async (method) => {
  const f = hardeningFixture(); f.state.heldHost = "second"; const parent = new AbortController();
  const pending = method === "listNetworkSummaries" ? f.adapter[method](parent.signal) : f.adapter[method](undefined, parent.signal);
  const failed = pending.catch((error: unknown) => error);
  await vi.waitFor(() => expect(f.state.heldSignals.length > 0).toBe(true));
  parent.abort(); await failed;
  expect(f.state.heldSignals.every((s) => s.aborted)).toBe(true);
});

it("PX-B5 pagination and a held response body consume the same original host budget", async () => {
  vi.useFakeTimers(); const f = hardeningFixture();
  const original = f.fetcher.getMockImplementation()!; const firstPage = deferred<void>();
  const reached = deferred<void>(); const bodyStarted = deferred<void>(); let bodySignal!: AbortSignal;
  f.fetcher.mockImplementation(async (input, init) => {
    const path = String(input);
    const result = await original(input, init);
    if (!path.includes("/environments/second/networks?")) return result;
    if (path.includes("start=0&")) {
      reached.resolve(); await firstPage.promise;
      const body = await result.json(); body.data = Array.from({ length: 100 }, () => body.data[0]); body.pagination.totalItems = 101;
      return Response.json(body);
    }
    bodySignal = init!.signal!;
    return new Response(new ReadableStream({ start(controller) {
      bodyStarted.resolve(); bodySignal.addEventListener("abort", () => controller.error(bodySignal.reason), { once: true });
    } }));
  });
  const pending = f.adapter.listNetworkSummaries(); let settled = false; void pending.then(() => { settled = true; });
  await reached.promise; await vi.advanceTimersByTimeAsync(20_000); firstPage.resolve(); await bodyStarted.promise;
  await vi.advanceTimersByTimeAsync(9_999); expect(settled).toBe(false); expect(bodySignal.aborted).toBe(false);
  await vi.advanceTimersByTimeAsync(1); expect(bodySignal.aborted).toBe(true);
  expect(await pending).toEqual([{ id: "net", hostId: "first" }]);
});
it("PX-B5 backup decoration receives only the remainder of the volume query budget", async () => {
  vi.useFakeTimers(); const f = hardeningFixture(); const original = f.fetcher.getMockImplementation()!;
  const first = deferred<void>(), reached = deferred<void>(), decoration = deferred<void>(); let signal!: AbortSignal;
  f.fetcher.mockImplementation(async (input, init) => {
    const path = String(input);
    if (path.includes("/environments/second/volumes?")) { reached.resolve(); await first.promise; }
    if (path.includes("/environments/second/volumes/backups/overview")) {
      signal = init!.signal!; decoration.resolve(); return new Promise<Response>((_resolve, reject) => signal.addEventListener("abort", () => reject(signal.reason), { once: true }));
    }
    return original(input, init);
  });
  const pending = f.adapter.listVolumes(); let settled = false; void pending.then(() => { settled = true; });
  await reached.promise; await vi.advanceTimersByTimeAsync(20_000); first.resolve(); await decoration.promise;
  await vi.advanceTimersByTimeAsync(9_999); expect(settled).toBe(false);
  await vi.advanceTimersByTimeAsync(1); expect(signal.aborted).toBe(true); await pending; expect(settled).toBe(true);
});

it("PX-B5 a matching volume crumb detail is bounded through its backup-history body", async () => {
  vi.useFakeTimers(); const f = hardeningFixture(); const original = f.fetcher.getMockImplementation()!;
  const reached = deferred<void>(); let signal!: AbortSignal;
  f.fetcher.mockImplementation(async (input, init) => {
    if (String(input).includes("/volumes/data/backups?")) {
      signal = init!.signal!; reached.resolve();
      return new Promise<Response>((_resolve, reject) => signal.addEventListener("abort", () => reject(signal.reason), { once: true }));
    }
    return original(input, init);
  });
  const pending = f.adapter.getVolume("first~data"); const failed = pending.catch((error: unknown) => error);
  await reached.promise; await vi.advanceTimersByTimeAsync(30_000);
  expect(await failed).toMatchObject({ outcome: { code: "http.error" } }); expect(signal.aborted).toBe(true);
});
it("PX-B5 an early host failure cancels its held sibling without affecting a healthy host", async () => {
  const f = hardeningFixture(); f.state.heldHost = "second"; f.state.holdPhase = "/containers";
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input, init) => String(input).includes("/environments/second/projects?") ? Response.json({ code: "http.error" }, { status: 500 }) : original(input, init));
  const rows = await f.adapter.listStacks();
  expect(rows.map((row) => row.hostId)).toEqual(["first"]);
  expect(f.state.heldSignals.length).toBe(1); expect(f.state.heldSignals[0]!.aborted).toBe(true);
});
