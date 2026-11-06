// Source: docklet-next/issues/349
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original behavioural break-map proofs. M19 web consumer integration.

import { expect, it, vi } from "vitest";
import type { DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import { runBulk, readBulkUpdates, type BulkItem } from "./bulk-run";
const item = (id: string): BulkItem => ({ id, name: `Stack ${id}`, host: "Example host", kind: "stack" });
function barrier<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((done) => { resolve = done; }); return { promise, resolve }; }
function fixture() {
  return { subscribeChanges: () => () => {}, getCapability: vi.fn<DockletAdapter["getCapability"]>(async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true })), admitRuntimeCommand: vi.fn(async () => ({ outcome: "started" as const, runId: "run" })) } as unknown as DockletAdapter;
}
it("partial failure retains busy, offline and successful item outcomes", async () => {
  const adapter = fixture();
  adapter.admitRuntimeCommand = vi.fn(async (request) => { if (request.id !== "ok") throw new AdapterOutcomeError({ kind: "error", code: request.id === "busy" ? "run.busy" : "agent.unavailable" }); return { outcome: "started" as const, runId: "accepted" }; });
  const result = await runBulk(adapter, [item("busy"), item("offline"), item("ok")], "start");
  expect(result.map((row) => [row.id, row.outcome, row.code])).toEqual([["busy", "failed", "run.busy"], ["offline", "failed", "agent.unavailable"], ["ok", "started", undefined]]);
});
it("captured snapshot survives clear and renamed refetch before late admission", async () => {
  const adapter = fixture(), held = barrier<{ outcome: "started"; runId: string }>();
  adapter.admitRuntimeCommand = () => held.promise;
  const selection = [item("slow")];
  const done = runBulk(adapter, selection, "restart");
  await Promise.resolve(); selection[0]!.name = "Renamed after refetch"; selection.length = 0;
  held.resolve({ outcome: "started" as const, runId: "accepted" });
  expect((await done)[0]?.name).toBe("Stack slow");
});
it("identity change stops queued items and reports session ended", async () => {
  const adapter = fixture(), held = barrier<{ outcome: "started"; runId: string }>(); let version = 0;
  registerIdentityLifetime(adapter.subscribeChanges, { version: () => version, active: () => version === 0, listen: () => () => {} });
  const send = vi.fn(() => held.promise); adapter.admitRuntimeCommand = send;
  const done = runBulk(adapter, Array.from({ length: 9 }, (_, i) => item(String(i))), "start");
  await vi.waitFor(() => expect(send).toHaveBeenCalledTimes(4)); version++;
  held.resolve({ outcome: "started" as const, runId: "accepted" });
  expect((await done).every((row) => row.outcome === "failed" && row.code === "session.ended")).toBe(true);
  expect(send).toHaveBeenCalledTimes(4);
});
it("restricted items send zero requests for redeploy and updates", async () => {
  const adapter = fixture(); adapter.getCapability = vi.fn(async (operationId, resourceRef) => ({ operationId, resourceRef, available: operationId === "runStackCommand", disabledReasonCode: "access.full-required" }));
  for (const action of ["redeploy", "update"] as const) expect((await runBulk(adapter, [item("restricted")], action))[0]).toMatchObject({ outcome: "skipped", code: "access.full-required" });
  expect(adapter.admitRuntimeCommand).toHaveBeenCalledTimes(0);
  expect((await runBulk(adapter, [item("restricted")], "start"))[0]?.outcome).toBe("started");
});
it("per-action preflight uses redeployStack, installStackUpdates and container grants", async () => {
  const adapter = fixture();
  await runBulk(adapter, [item("one")], "redeploy"); await runBulk(adapter, [item("one")], "update", new Map([["one", "revision"]])); await runBulk(adapter, [{ ...item("container"), kind: "container" }], "restart");
  expect(vi.mocked(adapter.getCapability).mock.calls.map(([op]) => op)).toEqual(["redeployStack", "installStackUpdates", "runContainerCommand"]);
});
it("concurrency cap holds four admissions until each finishes", async () => {
  const adapter = fixture(); let active = 0, maximum = 0, completed = 0;
  const pending: (() => void)[] = [];
  adapter.admitRuntimeCommand = async () => { active++; maximum = Math.max(maximum, active); await new Promise<void>((resolve) => pending.push(resolve)); active--; completed++; return { outcome: "started" as const, runId: "run" }; };
  const done = runBulk(adapter, Array.from({ length: 11 }, (_, i) => item(String(i))), "start");
  await vi.waitFor(() => expect(active).toBeGreaterThan(0));
  expect(maximum).toBe(4);
  while (completed < 11) { pending.splice(0).forEach((resolve) => resolve()); await new Promise((resolve) => setTimeout(resolve, 0)); }
  expect((await done).length).toBe(11); expect(maximum).toBe(4);
});
it("bulk update eligibility uses policy candidates and baseline only for savedRevision", async () => {
  const adapter = fixture();
  adapter.getAutoUpdatePolicy = vi.fn(async (id) => ({ stale: id === "stale", candidates: id === "current" ? [] : [{}] })) as unknown as DockletAdapter["getAutoUpdatePolicy"];
  adapter.getUpdateBaseline = vi.fn(async () => ({ available: true, savedRevision: "revision-A" })) as unknown as DockletAdapter["getUpdateBaseline"];
  const rows = await readBulkUpdates(adapter, [item("current"), item("candidate"), item("stale"), item("race")]);
  expect(rows.map((row) => [row.item.id, row.revision, row.code])).toEqual([["current", undefined, "bulk.up-to-date"], ["candidate", "revision-A", undefined], ["stale", undefined, "bulk.not-checked"], ["race", "revision-A", undefined]]);
  expect(vi.mocked(adapter.getUpdateBaseline!).mock.calls.map(([id]) => id)).toEqual(["candidate", "race"]);
  adapter.admitRuntimeCommand = vi.fn(async (request) => { expect(request.input?.expectedRevision).toBe("revision-A"); if (request.id === "race") throw new AdapterOutcomeError({ kind: "conflict", code: "run.request-mismatch" }); return { outcome: "joined" as const, runId: "existing" }; });
  const eligible = rows.filter((row) => row.revision);
  const result = await runBulk(adapter, eligible.map((row) => row.item), "update", new Map(eligible.map((row) => [row.item.id, row.revision!])));
  expect(result.map((row) => [row.outcome, row.code])).toEqual([["joined", undefined], ["failed", "run.request-mismatch"]]);
});
