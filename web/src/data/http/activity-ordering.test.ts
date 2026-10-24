// Source: docklet-next/issues/102
// Revision: 28f968e594b730c4960f57296abf7c44158c28c5
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original deterministic activity ordering and removed-source health regressions; full/full overlap cases replaced by coalescing coverage (#211).

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
it("newer targeted failure survives an older full success and another health publication", async () => {
  const f = fixture(); await f.reader.load();
  const held = deferred<{ items: ReturnType<typeof activity>[]; incomplete: boolean }>(); const reached = deferred<void>();
  f.reads.page.mockImplementation(async (hostId) => {
    if (hostId === "bad") { reached.resolve(); return held.promise; }
    return { items: [activity(hostId)], incomplete: false };
  });
  const older = f.reader.load(); await reached.promise;
  f.reads.detail.mockRejectedValueOnce(unavailable());
  await f.reader.refreshOne("run-bad", "bad");
  held.resolve({ items: [activity("bad")], incomplete: false }); await older;
  await publishOtherFailure(f);
  expect(f.reader.health().failures).toEqual([
    { hostId: "bad", reason: "unavailable" }, { hostId: "good", reason: "unavailable" },
  ]);
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

it("an opened detail keeps newer list status while preserving separately loaded output", async () => {
  const f = fixture(); await f.reader.load(); const held = deferred<Detail>(); f.reads.detail.mockReturnValueOnce(held.promise);
  const pending = f.reader.readDetail("run-good", "good", undefined, true);
  f.reads.page.mockImplementation(async (host) => ({ items: [{ ...activity(host), status: "queued" }], incomplete: false }));
  await f.reader.load(); const stale = detail("good"); stale.output = [{ timestamp: stale.activity.startedAt, stream: "stdout", text: "retained output" }]; held.resolve(stale);
  expect(await pending).toMatchObject({ status: "queued", output: [{ text: "retained output" }] });
  expect(f.reader.runs().find((run) => run.id === "run-good")).toMatchObject({ status: "queued", output: [] });
});
