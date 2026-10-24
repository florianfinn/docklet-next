// Source: docklet-next/issues/211
// Revision: 3cba55fd10c311e4a5b3361afabb8c93fca706a0
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original controlled discovery concurrency, bounded trailing refreshes and deterministic identity guards.

import { CancelledError } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import { createActivityReader } from "./activity-reader";
import { activity, deferred, detail } from "./activity-reader.test-support";

function fixture() {
  const reads = {
    hosts: vi.fn(async () => ["good", "other"]),
    page: vi.fn(async (host: string) => ({ items: Array.from({ length: 12 }, (_, i) => activity(host, `${host}-${i}`)), incomplete: false })),
    detail: vi.fn(async (host: string, id: string) => detail(host, id)),
    publish: vi.fn(), unauthorized: vi.fn(),
  };
  return { reads, reader: createActivityReader(reads) };
}

it("bounds discovery details to four per host and advances every host independently", async () => {
  const f = fixture();
  const gates = { good: deferred<void>(), other: deferred<void>() };
  const reached = { good: deferred<void>(), other: deferred<void>() };
  const active: Record<string, number> = { good: 0, other: 0 };
  const max: Record<string, number> = { good: 0, other: 0 };
  f.reads.detail.mockImplementation(async (host, id) => {
    active[host] = active[host]! + 1; max[host] = Math.max(max[host]!, active[host]!);
    if (active[host] === 4) reached[host as keyof typeof reached].resolve();
    await gates[host as keyof typeof gates].promise;
    active[host] = active[host]! - 1;
    return detail(host, id);
  });
  const pending = f.reader.load();
  await Promise.all([reached.good.promise, reached.other.promise]);
  expect(f.reads.detail).toHaveBeenCalledTimes(8);
  gates.other.resolve();
  // Completion of the other host is observable while the first remains held.
  await vi.waitFor(() => expect(f.reads.detail.mock.calls.filter(([host]) => host === "other")).toHaveLength(12));
  expect(active.good).toBe(4);
  gates.good.resolve();
  expect(await pending).toHaveLength(24);
  expect(max).toEqual({ good: 4, other: 4 });
});

it("coalesces concurrent full loads without a second discovery", async () => {
  const f = fixture(); const gate = deferred<string[]>();
  f.reads.hosts.mockReturnValueOnce(gate.promise);
  const first = f.reader.load(); const second = f.reader.load();
  expect(second).toBe(first);
  expect(f.reads.hosts).toHaveBeenCalledTimes(1);
  gate.resolve(["good", "other"]);
  expect(await second).toHaveLength(24);
  expect(f.reads.page).toHaveBeenCalledTimes(2);
});

it("coalesces notifications during a load into one trailing refresh of post-event state", async () => {
  const f = fixture(); const gate = deferred<void>(); const reached = deferred<void>();
  f.reads.hosts.mockResolvedValue(["good"]);
  f.reads.page.mockResolvedValue({ items: [activity("good")], incomplete: false });
  f.reads.detail.mockImplementationOnce(async () => { reached.resolve(); await gate.promise; return detail("good"); });
  const fresh = detail("good"); fresh.activity.status = "queued";
  f.reads.detail.mockResolvedValue(fresh);
  const first = f.reader.load(); await reached.promise;
  const trailing = f.reader.refresh();
  expect(f.reader.refresh()).toBe(trailing);
  expect(trailing).not.toBe(first);
  expect(f.reads.hosts).toHaveBeenCalledTimes(1);
  gate.resolve();
  expect((await first)[0]?.status).toBe("running");
  expect((await trailing)[0]?.status).toBe("queued");
  expect(f.reads.hosts).toHaveBeenCalledTimes(2);
  expect(f.reads.detail).toHaveBeenCalledTimes(2);
});

it("keeps a known run fresh without waiting behind discovery details", async () => {
  const f = fixture();
  f.reads.hosts.mockResolvedValue(["good"]);
  await f.reader.load();
  const gate = deferred<void>(); const reached = deferred<void>(); let started = 0;
  f.reads.detail.mockImplementation(async (host, id) => {
    const old = detail(host, id);
    if (++started <= 4) { if (started === 4) reached.resolve(); await gate.promise; return old; }
    const fresh = detail(host, id); fresh.activity.status = "queued"; return fresh;
  });
  const full = f.reader.load(); await reached.promise;
  await f.reader.refreshOne("good-0", "good");
  expect(f.reader.runs().find((run) => run.id === "good-0")?.status).toBe("queued");
  gate.resolve(); await full;
  expect(f.reader.runs().find((run) => run.id === "good-0")?.status).toBe("queued");
});

it("shares failures and evicts failed full work for explicit retry", async () => {
  const f = fixture(); const gate = deferred<string[]>();
  f.reads.hosts.mockReturnValueOnce(gate.promise);
  const first = f.reader.load(); const second = f.reader.load();
  const rejection = expect(first).rejects.toThrow("unavailable");
  expect(second).toBe(first);
  gate.reject(new Error("unavailable")); await rejection;
  expect(f.reader.health().failures).toEqual([{ hostId: undefined, reason: "unavailable" }]);
  expect(await f.reader.load()).toHaveLength(24);
  expect(f.reader.health().failures).toEqual([]);
});

it("new identity work does not join an old full read or publish its late result", async () => {
  const f = fixture(); const gate = deferred<string[]>();
  f.reads.hosts.mockReturnValueOnce(gate.promise);
  const old = f.reader.load(); const rejection = old.catch((error: unknown) => error);
  f.reader.reset();
  const fresh = f.reader.load();
  expect(fresh).not.toBe(old);
  expect(await fresh).toHaveLength(24);
  const publications = f.reads.publish.mock.calls.length;
  gate.resolve([]); expect(await rejection).toBeInstanceOf(CancelledError);
  expect(f.reader.runs()).toHaveLength(24);
  expect(f.reads.publish).toHaveBeenCalledTimes(publications);
});

it("recovery queues authentication validation behind held discovery and clears its observations", async () => {
  const f = fixture(); await f.reader.load();
  const gate = deferred<ReturnType<typeof detail>>(); const reached = deferred<void>();
  f.reads.detail.mockImplementationOnce(async () => { reached.resolve(); return gate.promise; });
  const old = f.reader.load(); await reached.promise;
  const { AdapterOutcomeError } = await import("../capability");
  f.reads.hosts.mockRejectedValueOnce(new AdapterOutcomeError({ kind: "error", code: "http.error", status: 401 }));
  const fresh = f.reader.refresh();
  expect(fresh).not.toBe(old);
  expect(f.reads.hosts).toHaveBeenCalledTimes(2);
  gate.resolve(detail("good", "good-0")); await old;
  await expect(fresh).rejects.toMatchObject({ outcome: { status: 401 } });
  expect(f.reader.runs()).toEqual([]);
  expect(f.reads.hosts).toHaveBeenCalledTimes(3);
});
