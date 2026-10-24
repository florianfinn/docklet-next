// Source: docklet-next/issues/211
// Revision: 79ee005c9dbf2fb261bd413a6d273c4cce71c2e3
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original renewal credential replay isolation for full observations, source health and host membership.

import { expect, it, vi } from "vitest";
import { AdapterOutcomeError } from "../capability";
import { createActivityReader } from "./activity-reader";
import { activity, deferred, detail, type Detail } from "./activity-reader.test-support";

const unavailable = () => new AdapterOutcomeError({ kind: "error", code: "http.error", status: 502 });
async function fixture() {
  const reads = {
    hosts: vi.fn(async () => ["good", "bad"]),
    page: vi.fn(async (host: string) => ({ items: [activity(host)], incomplete: false })),
    detail: vi.fn(async (host: string, id: string) => detail(host, id)),
    publish: vi.fn(), unauthorized: vi.fn(),
  };
  const reader = createActivityReader(reads); await reader.load();
  return { reads, reader };
}
async function publishOtherFailure(f: Awaited<ReturnType<typeof fixture>>) {
  f.reads.detail.mockRejectedValueOnce(unavailable());
  await f.reader.refreshOne("run-good", "good");
}

it.each([true, false])("renewal newer full health ignores superseded credential observation (old failure=%s)", async (oldFailure) => {
  const f = await fixture(); const held = deferred<Detail>(); const reached = deferred<void>();
  f.reads.detail.mockImplementation(async (host, id) => {
    if (host === "bad") { reached.resolve(); return held.promise; }
    return detail(host, id);
  });
  const older = f.reader.load().catch((e: unknown) => e); await reached.promise;
  f.reader.invalidate(true);
  f.reads.detail.mockImplementation(async (host, id) => {
    if (host === "bad" && !oldFailure) throw unavailable();
    return detail(host, id);
  });
  const fresh = f.reader.load();
  held.resolve(oldFailure ? {} as Detail : detail("bad"));
  expect(await fresh).toHaveLength(2);
  expect(await older).toHaveLength(2);
  expect(f.reads.hosts).toHaveBeenCalledTimes(3);
  await publishOtherFailure(f);
  expect(f.reader.health().failures).toEqual(oldFailure
    ? [{ hostId: "good", reason: "unavailable" }]
    : [{ hostId: "bad", reason: "unavailable" }, { hostId: "good", reason: "unavailable" }]);
});

it.each(["empty source", "removed host"])("renewal %s blocks unseen runs in superseded credential work", async (change) => {
  const f = await fixture(); const held = deferred<Detail>(); const reached = deferred<void>();
  f.reads.page.mockImplementation(async (host) => ({ items: [activity(host, host === "bad" ? "unseen" : "run-good")], incomplete: false }));
  f.reads.detail.mockImplementation(async (host, id) => {
    if (host === "bad") { reached.resolve(); return held.promise; }
    return detail(host, id);
  });
  const older = f.reader.load().catch((e: unknown) => e); await reached.promise; f.reader.invalidate(true);
  if (change === "removed host") f.reads.hosts.mockResolvedValue(["good"]);
  f.reads.page.mockImplementation(async (host) => ({ items: host === "bad" ? [] : [activity(host)], incomplete: false }));
  const fresh = f.reader.load(); held.resolve(detail("bad", "unseen"));
  expect((await fresh).map((run) => run.id)).toEqual(["run-good"]);
  expect((await older as { id: string }[]).map((run) => run.id)).toEqual(["run-good"]);
  await publishOtherFailure(f);
  expect(f.reader.runs().map((run) => run.id)).toEqual(["run-good"]);
});

it.each([true, false])("renewal global health ignores superseded credential observation (old failure=%s)", async (oldFailure) => {
  const f = await fixture(); const hosts = deferred<string[]>();
  f.reads.hosts.mockReturnValueOnce(hosts.promise);
  const older = f.reader.load().catch((e: unknown) => e); f.reader.invalidate(true);
  if (!oldFailure) f.reads.hosts.mockRejectedValueOnce(unavailable());
  const fresh = f.reader.load().catch((e: unknown) => e);
  const reads = f.reads.page.mock.calls.length;
  if (oldFailure) hosts.reject(unavailable()); else hosts.resolve(["good", "bad"]);
  if (oldFailure) expect(await older).toHaveLength(2);
  else expect(await older).toBeInstanceOf(AdapterOutcomeError);
  await fresh;
  expect(f.reads.page).toHaveBeenCalledTimes(reads + (oldFailure ? 2 : 0));
  await publishOtherFailure(f);
  expect(f.reader.health().failures).toEqual(oldFailure
    ? [{ hostId: "good", reason: "unavailable" }]
    : [{ hostId: undefined, reason: "unavailable" }, { hostId: "good", reason: "unavailable" }]);
});

it("renewal superseded source health preserves a newer global failure", async () => {
  const f = await fixture(); const held = deferred<Detail>(); const reached = deferred<void>();
  f.reads.detail.mockImplementation(async (host, id) => {
    if (host === "bad") { reached.resolve(); return held.promise; }
    return detail(host, id);
  });
  const older = f.reader.load().catch((e: unknown) => e); await reached.promise; f.reader.invalidate(true);
  f.reads.hosts.mockRejectedValueOnce(unavailable());
  const fresh = f.reader.load();
  held.reject(unavailable());
  await expect(fresh).rejects.toBeInstanceOf(AdapterOutcomeError);
  expect(await older).toBeInstanceOf(AdapterOutcomeError);
  expect(f.reader.health().failures).toEqual([{ hostId: undefined, reason: "unavailable" }]);
  expect(f.reads.publish.mock.lastCall?.[1]).toEqual(f.reader.health());
});
