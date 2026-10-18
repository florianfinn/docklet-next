// Source: docklet-next/issues/102
// Revision: 28f968e594b730c4960f57296abf7c44158c28c5
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original deterministic activity ordering and removed-source health regressions.

import { expect, it, vi } from "vitest";
import { AdapterOutcomeError } from "../capability";
import { createActivityReader } from "./activity-reader";
import { activity, deferred, detail, type Detail } from "./activity-reader.test-support";

const unavailable = () => new AdapterOutcomeError({ kind: "error", code: "http.error", status: 502 });
function fixture() {
  const reads = {
    hosts: vi.fn(async () => ["good", "bad"]),
    page: vi.fn(async (hostId: string) => ({ items: [activity(hostId)], incomplete: false })),
    detail: vi.fn(async (hostId: string, id: string) => detail(hostId, id)),
    publish: vi.fn(), unauthorized: vi.fn(),
  };
  return { reads, reader: createActivityReader(reads) };
}
// Publish another source's health without rereading the source whose map may be corrupt.
async function publishOtherFailure(f: ReturnType<typeof fixture>) {
  f.reads.detail.mockRejectedValueOnce(unavailable());
  await f.reader.refreshOne("run-good", "good");
}
it.each([true, false])("completed newer full read wins over older full health (old failure=%s)", async (oldFailure) => {
  const f = fixture(); await f.reader.load();
  const held = deferred<Detail>(); const reached = deferred<void>();
  f.reads.detail.mockImplementation(async (hostId, id) => {
    if (hostId === "bad") { reached.resolve(); return held.promise; }
    return detail(hostId, id);
  });
  const older = f.reader.load();
  const settled = older.catch(() => undefined);
  await reached.promise;
  f.reads.detail.mockImplementation(async (hostId, id) => {
    if (hostId === "bad" && !oldFailure) throw unavailable();
    return detail(hostId, id);
  });
  await f.reader.load(); // Explicit newer completion, never an already-true predicate.
  if (oldFailure) held.resolve({} as Detail); else held.resolve(detail("bad"));
  await settled;
  await publishOtherFailure(f);
  expect(f.reader.health().failures).toEqual(oldFailure
    ? [{ hostId: "good", reason: "unavailable" }]
    : [{ hostId: "bad", reason: "unavailable" }, { hostId: "good", reason: "unavailable" }]);
});
it("newer targeted failure survives an older full success and another health publication", async () => {
  const f = fixture(); await f.reader.load();
  const held = deferred<Detail>(); const reached = deferred<void>();
  f.reads.detail.mockImplementation(async (hostId, id) => {
    if (hostId === "bad") { reached.resolve(); return held.promise; }
    return detail(hostId, id);
  });
  const older = f.reader.load(); await reached.promise;
  f.reads.detail.mockRejectedValueOnce(unavailable());
  await f.reader.refreshOne("run-bad", "bad");
  held.resolve(detail("bad")); await older;
  await publishOtherFailure(f);
  expect(f.reader.health().failures).toEqual([
    { hostId: "bad", reason: "unavailable" }, { hostId: "good", reason: "unavailable" },
  ]);
});
it("a completed empty full read blocks a previously unseen run in an older full read", async () => {
  const f = fixture(); await f.reader.load();
  const held = deferred<Detail>(); const reached = deferred<void>();
  f.reads.page.mockImplementation(async (hostId) => ({ items: [activity(hostId, hostId === "bad" ? "not-yet-observed" : "run-good")], incomplete: false }));
  f.reads.detail.mockImplementation(async (hostId, id) => {
    if (hostId === "bad") { reached.resolve(); return held.promise; }
    return detail(hostId, id);
  });
  const older = f.reader.load(); await reached.promise;
  f.reads.page.mockImplementation(async (hostId) => ({ items: hostId === "bad" ? [] : [activity(hostId)], incomplete: false }));
  await f.reader.load();
  held.resolve(detail("bad", "not-yet-observed")); await older;
  await publishOtherFailure(f);
  expect(f.reader.runs().map((r) => r.id)).toEqual(["run-good"]);
});
it("authoritative discovery removes a previously failing host's warning as well as its runs", async () => {
  const f = fixture(); await f.reader.load();
  f.reads.page.mockImplementation(async (hostId) => {
    if (hostId === "bad") throw unavailable();
    return { items: [activity(hostId)], incomplete: false };
  });
  await f.reader.load();
  expect(f.reader.health().failures).toEqual([{ hostId: "bad", reason: "unavailable" }]);
  f.reads.hosts.mockResolvedValue(["good"]);
  await f.reader.load();
  await publishOtherFailure(f);
  expect(f.reader.runs().map((r) => r.id)).toEqual(["run-good"]);
  expect(f.reader.health().failures).toEqual([{ hostId: "good", reason: "unavailable" }]);
});
it.each([true, false])("global health obeys newer full completion (old failure=%s)", async (oldFailure) => {
  const f = fixture(); await f.reader.load();
  const held = deferred<string[]>();
  f.reads.hosts.mockReturnValueOnce(held.promise);
  const older = f.reader.load(); const settled = older.catch(() => undefined);
  if (oldFailure) await f.reader.load();
  else { f.reads.hosts.mockRejectedValueOnce(unavailable()); await expect(f.reader.load()).rejects.toMatchObject({ outcome: { status: 502 } }); }
  if (oldFailure) held.reject(unavailable()); else held.resolve(["good", "bad"]);
  await settled;
  await publishOtherFailure(f);
  expect(f.reader.health().failures).toEqual(oldFailure
    ? [{ hostId: "good", reason: "unavailable" }]
    : [{ hostId: undefined, reason: "unavailable" }, { hostId: "good", reason: "unavailable" }]);
});
it.each([true, false])("newer full completion suppresses an older targeted result (failure=%s)", async (failed) => {
  const f = fixture(); await f.reader.load();
  const held = deferred<Detail>();
  f.reads.detail.mockReturnValueOnce(held.promise);
  const older = f.reader.refreshOne("run-bad", "bad");
  f.reads.page.mockImplementation(async (hostId) => ({ items: hostId === "bad" ? [] : [activity(hostId)], incomplete: false }));
  await f.reader.load();
  held.resolve(failed ? {} as Detail : detail("bad")); await older;
  await publishOtherFailure(f);
  expect(f.reader.runs().map((r) => r.id)).toEqual(["run-good"]);
  expect(f.reader.health().failures).toEqual([{ hostId: "good", reason: "unavailable" }]);
});
it("an accepted source result republishes source health while preserving a newer global failure", async () => {
  const f = fixture(); await f.reader.load();
  const held = deferred<Detail>(); const reached = deferred<void>();
  f.reads.detail.mockImplementation(async (hostId, id) => {
    if (hostId === "bad") { reached.resolve(); return held.promise; }
    return detail(hostId, id);
  });
  const older = f.reader.load(); await reached.promise;
  f.reads.hosts.mockRejectedValueOnce(unavailable());
  await expect(f.reader.load()).rejects.toMatchObject({ outcome: { status: 502 } });
  held.reject(unavailable()); await older;
  expect(f.reader.health().failures).toEqual([
    { hostId: undefined, reason: "unavailable" }, { hostId: "bad", reason: "unavailable" },
  ]);
  expect(f.reads.publish.mock.lastCall?.[1]).toEqual(f.reader.health());
});
