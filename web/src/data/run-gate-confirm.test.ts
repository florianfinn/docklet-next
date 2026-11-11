// Source: docklet-next/issues/431
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proofs that the run gate confirms a stale blocker with one plain hub read (#431, C2-d..g).

import { CancelledError } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import type { DockletAdapter } from "./adapter";
import { createDisconnectedAdapter } from "./disconnected-adapter";
import type { CodedActionRun } from "./message-code";
import { RunGate, isRunBusy } from "./run-gate";
import { mapRuntime } from "./http/runtime-mapping";
import { RUN_ID, runtimeFixture, snapshot } from "./http/runtime-commands.test-support";

/** A gate over a fake run list whose blocker confirmation is the real runtime HTTP read. */
function fixture() {
  const runtime = runtimeFixture();
  let push!: (runs: CodedActionRun[]) => void;
  const inner: DockletAdapter = { ...createDisconnectedAdapter(), kind: "http", mappedOperations: new Set(["runStackCommand", "runContainerCommand"]),
    subscribeActions: (listener) => { push = listener; return () => {}; },
    runStackCommand: vi.fn(async () => "stack-run"), runContainerCommand: vi.fn(async (id) => `container-${id}`),
    confirmRun: vi.fn((host: string, id: string) => runtime.helper.confirmRun(host, id)),
  };
  const gate = new RunGate(inner);
  // A local row still says running; the hub's record decides.
  const staleRow = mapRuntime(snapshot("edge", { resourceType: "container", resourceId: "a" }), "edge");
  const reads = () => runtime.calls.filter(({ url }) => url.pathname.endsWith(`/runtime-runs/${RUN_ID}`));
  return { gate, inner, runtime, reads, emit: (runs: CodedActionRun[]) => push(runs), staleRow };
}
const busy = (promise: Promise<unknown>) => promise.then(() => "started", (error: unknown) => isRunBusy(error) ? "busy" : error);

it("C2-d a stale local running row whose hub record is terminal: one plain confirmation read, then one command", async () => {
  const f = fixture();
  f.runtime.state.run = snapshot("edge", { resourceType: "container", resourceId: "a", status: "success", outcomeState: "known", completedAt: "2026-11-11T12:00:00Z" } as never);
  await f.gate.run("runStackCommand", ["edge~other", "start"]); f.emit([f.staleRow]);
  const outcome = await busy(f.gate.run("runContainerCommand", ["edge~a", "restart"]));
  expect([outcome, f.reads().length, f.reads().some(({ url }) => url.search.includes("refresh")), vi.mocked(f.inner.runContainerCommand).mock.calls.length]).toEqual(["started", 1, false, 1]);
});

it.each([
  ["running", { status: "running", outcomeState: "known" }],
  ["queued", { status: "queued", outcomeState: "known" }],
  ["outcome unknown", { status: "failed", outcomeState: "unknown" }],
])("C2-e a confirmation answering %s refuses and sends nothing", async (_case, patch) => {
  const f = fixture();
  f.runtime.state.run = snapshot("edge", { resourceType: "container", resourceId: "a", ...patch } as never);
  await f.gate.run("runStackCommand", ["edge~other", "start"]); f.emit([f.staleRow]);
  expect(await busy(f.gate.run("runContainerCommand", ["edge~a", "restart"]))).toBe("busy");
  expect([f.reads().length, vi.mocked(f.inner.runContainerCommand).mock.calls.length]).toEqual([1, 0]);
});

it("C2-e a failed confirmation refuses; a later click confirms again and starts once the hub says terminal", async () => {
  const f = fixture();
  await f.gate.run("runStackCommand", ["edge~other", "start"]); f.emit([f.staleRow]);
  f.runtime.state.detailStatus = 404;
  expect(await busy(f.gate.run("runContainerCommand", ["edge~a", "restart"]))).toBe("busy");
  f.runtime.state.detailStatus = 200;
  f.runtime.state.run = snapshot("edge", { resourceType: "container", resourceId: "a", status: "success", outcomeState: "known" } as never);
  const later = await busy(f.gate.run("runContainerCommand", ["edge~a", "restart"]));
  expect([later, f.reads().length, vi.mocked(f.inner.runContainerCommand).mock.calls.length]).toEqual(["started", 2, 1]);
});

it("C2-f an identity reset during the confirmation cancels; nothing is sent", async () => {
  const f = fixture();
  let release!: () => void;
  const held = new Promise<void>((resolve) => { release = resolve; });
  f.runtime.state.read = async () => { await held; return new Response(JSON.stringify({ success: true, data: { activity: snapshot("edge", { status: "success", outcomeState: "known" } as never), messages: [], stepTrace: [], output: [], outputTruncated: false, retainedStartSeq: 0, recovery: { executorState: "pending", releaseAcknowledged: false } } }), { headers: { "Content-Type": "application/json" } }); };
  await f.gate.run("runStackCommand", ["edge~other", "start"]); f.emit([f.staleRow]);
  const attempt = f.gate.run("runContainerCommand", ["edge~a", "restart"]).then(() => "started", (error: unknown) => error);
  await vi.waitFor(() => expect(f.reads().length).toBe(1));
  f.gate.reset(); release();
  expect([(await attempt) instanceof CancelledError, vi.mocked(f.inner.runContainerCommand).mock.calls.length]).toEqual([true, 0]);
});

it.each([["another run id", { id: "other-run" }], ["another host", { hostId: "direct" }]])("C2-f a confirmation answer for %s refuses", async (_case, patch) => {
  const f = fixture();
  f.inner.confirmRun = vi.fn(async () => ({ ...f.staleRow, status: "succeeded" as const, outcomeState: "known" as const, ...patch }));
  await f.gate.run("runStackCommand", ["edge~other", "start"]); f.emit([f.staleRow]);
  expect([await busy(f.gate.run("runContainerCommand", ["edge~a", "restart"])), vi.mocked(f.inner.runContainerCommand).mock.calls.length]).toEqual(["busy", 0]);
});

it("C2-g double click shares one confirmation and one start; a conflicting key waiting on the same read is refused by the complete re-admission", async () => {
  const f = fixture();
  f.runtime.state.run = snapshot("edge", { resourceType: "container", resourceId: "a", status: "success", outcomeState: "known" } as never);
  await f.gate.run("runStackCommand", ["edge~other", "start"]); f.emit([f.staleRow]);
  const [first, second, other] = await Promise.all([
    f.gate.run("runContainerCommand", ["edge~a", "restart"]),
    f.gate.run("runContainerCommand", ["edge~a", "restart"]),
    busy(f.gate.run("runContainerCommand", ["edge~a", "stop"])),
  ]);
  expect([first, second, other, vi.mocked(f.inner.confirmRun!).mock.calls.length, vi.mocked(f.inner.runContainerCommand).mock.calls.length]).toEqual(["container-edge~a", "container-edge~a", "busy", 1, 1]);
});

it("C2-g a new blocker that appears after the confirmation gets its own confirmation and refuses when it runs", async () => {
  const f = fixture();
  const second = { ...f.staleRow, id: "second-run" };
  f.inner.confirmRun = vi.fn(async (_host: string, id: string) => {
    // The first blocker ends; meanwhile a second run on the same target starts.
    if (id === f.staleRow.id) { f.emit([second]); return { ...f.staleRow, status: "succeeded" as const, outcomeState: "known" as const }; }
    return { ...second, outcomeState: "known" as const };
  });
  await f.gate.run("runStackCommand", ["edge~other", "start"]); f.emit([f.staleRow]);
  expect(await busy(f.gate.run("runContainerCommand", ["edge~a", "restart"]))).toBe("busy");
  expect([vi.mocked(f.inner.confirmRun!).mock.calls.map((call) => call[1]), vi.mocked(f.inner.runContainerCommand).mock.calls.length]).toEqual([[f.staleRow.id, "second-run"], 0]);
});

it.each([
  ["running", { status: "running", outcomeState: "known" }, "busy", 0, 1],
  ["queued", { status: "queued", outcomeState: "known" }, "busy", 0, 1],
  ["failed with an unknown outcome", { status: "failed", outcomeState: "unknown" }, "busy", 0, 1],
  ["succeeded", { status: "success", outcomeState: "known" }, "started", 1, 0],
])("C2-e through the HTTP adapter, whose read publishes the answer to the gate: %s", async (_case, patch, expected, sent, notices) => {
  const f = runtimeFixture();
  f.state.run = snapshot("edge", { resourceId: "a" });
  const gate = new RunGate(f.adapter);
  const busyNotices: string[] = [];
  gate.subscribe((notice) => { if (notice.reason === "busy") busyNotices.push(notice.key); });
  await gate.run("runContainerCommand", ["edge~a", "start"]);
  // The local row says running; the hub record now answers the case.
  await f.adapter.confirmRun!("edge", RUN_ID);
  const before = f.commands().length;
  f.state.run = snapshot("edge", { resourceId: "a", ...patch } as never);
  const outcome = await busy(gate.run("runContainerCommand", ["edge~a", "restart"]));
  gate.reset();
  expect([outcome, f.commands().length - before, busyNotices.length]).toEqual([expected, sent, notices]);
});

it("without a confirmation port (demo) the refusal stays synchronous as today", async () => {
  const f = fixture(); delete f.inner.confirmRun;
  await f.gate.run("runStackCommand", ["edge~other", "start"]); f.emit([f.staleRow]);
  expect(await busy(f.gate.run("runContainerCommand", ["edge~a", "restart"]))).toBe("busy");
  expect(f.reads().length).toBe(0);
});
