// Source: docklet-next/issues/211
// Revision: 79ee005c9dbf2fb261bd413a6d273c4cce71c2e3
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Restored full-read ordering regressions at reachable session-renewal overlap.

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
  const gate = deferred<string[]>(); reads.hosts.mockReturnValueOnce(gate.promise);
  const recovery = reader.load(); const settled = recovery.then((value) => ({ value }), (error: unknown) => ({ error }));
  reader.invalidate(true);
  return { reads, reader, resume: () => gate.resolve([]), settled };
}
async function publishOtherFailure(f: Awaited<ReturnType<typeof fixture>>) {
  f.reads.detail.mockRejectedValueOnce(unavailable());
  await f.reader.refreshOne("run-good", "good");
}

it.each([true, false])("renewal newer full health wins over an older full completion (old failure=%s)", async (oldFailure) => {
  const f = await fixture(); const held = deferred<Detail>(); const reached = deferred<void>();
  f.reads.detail.mockImplementation(async (host, id) => {
    if (host === "bad") { reached.resolve(); return held.promise; }
    return detail(host, id);
  });
  const older = f.reader.load(); await reached.promise;
  f.reads.detail.mockImplementation(async (host, id) => {
    if (host === "bad" && !oldFailure) throw unavailable();
    return detail(host, id);
  });
  f.resume(); expect(await f.settled).toHaveProperty("value");
  held.resolve(oldFailure ? {} as Detail : detail("bad")); await older;
  await publishOtherFailure(f);
  expect(f.reader.health().failures).toEqual(oldFailure
    ? [{ hostId: "good", reason: "unavailable" }]
    : [{ hostId: "bad", reason: "unavailable" }, { hostId: "good", reason: "unavailable" }]);
});

it("renewal empty newer full read blocks an unseen run in an older full read", async () => {
  const f = await fixture(); const held = deferred<Detail>(); const reached = deferred<void>();
  f.reads.page.mockImplementation(async (host) => ({ items: [activity(host, host === "bad" ? "unseen" : "run-good")], incomplete: false }));
  f.reads.detail.mockImplementation(async (host, id) => {
    if (host === "bad") { reached.resolve(); return held.promise; }
    return detail(host, id);
  });
  const older = f.reader.load(); await reached.promise;
  f.reads.page.mockImplementation(async (host) => ({ items: host === "bad" ? [] : [activity(host)], incomplete: false }));
  f.resume(); await f.settled;
  held.resolve(detail("bad", "unseen")); await older; await publishOtherFailure(f);
  expect(f.reader.runs().map((run) => run.id)).toEqual(["run-good"]);
});

it.each([true, false])("renewal global health obeys newer full completion (old failure=%s)", async (oldFailure) => {
  const f = await fixture(); const hosts = deferred<string[]>();
  f.reads.hosts.mockReturnValueOnce(hosts.promise);
  const older = f.reader.load(); const settled = older.catch(() => undefined);
  if (!oldFailure) f.reads.hosts.mockRejectedValueOnce(unavailable());
  f.resume(); await f.settled;
  if (oldFailure) hosts.reject(unavailable()); else hosts.resolve(["good", "bad"]);
  await settled; await publishOtherFailure(f);
  expect(f.reader.health().failures).toEqual(oldFailure
    ? [{ hostId: "good", reason: "unavailable" }]
    : [{ hostId: undefined, reason: "unavailable" }, { hostId: "good", reason: "unavailable" }]);
});

it("renewal older source health preserves a newer global failure", async () => {
  const f = await fixture(); const held = deferred<Detail>(); const reached = deferred<void>();
  f.reads.detail.mockImplementation(async (host, id) => {
    if (host === "bad") { reached.resolve(); return held.promise; }
    return detail(host, id);
  });
  const older = f.reader.load(); await reached.promise;
  f.reads.hosts.mockRejectedValueOnce(unavailable()); f.resume();
  expect(await f.settled).toHaveProperty("error");
  held.reject(unavailable()); await older;
  expect(f.reader.health().failures).toEqual([{ hostId: undefined, reason: "unavailable" }, { hostId: "bad", reason: "unavailable" }]);
  expect(f.reads.publish.mock.lastCall?.[1]).toEqual(f.reader.health());
});

it("renewal newer host discovery cannot be reversed by an older full source", async () => {
  const f = await fixture(); const held = deferred<Detail>(); const reached = deferred<void>();
  f.reads.page.mockImplementation(async (host) => ({ items: [activity(host, host === "bad" ? "unseen-host-run" : "run-good")], incomplete: false }));
  f.reads.detail.mockImplementation(async (host, id) => {
    if (host === "bad") { reached.resolve(); return held.promise; }
    return detail(host, id);
  });
  const older = f.reader.load(); await reached.promise;
  f.reads.hosts.mockResolvedValue(["good"]); f.resume(); await f.settled;
  held.resolve(detail("bad", "unseen-host-run")); await older;
  expect(f.reader.runs().map((run) => run.id)).toEqual(["run-good"]);
});
