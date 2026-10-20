// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { expect, it, vi } from "vitest";
import type { DockletAdapter } from "./adapter";
import { createDisconnectedAdapter } from "./disconnected-adapter";
import type { CodedActionRun } from "./message-code";
import { AdapterOutcomeError } from "./capability";
import { RunGate, isRunBusy } from "./run-gate";
import { mapRuntime } from "./http/runtime-mapping";
import { snapshot } from "./http/runtime-commands.test-support";

function fixture() {
  let push!: (runs: CodedActionRun[]) => void;
  const stop = vi.fn(); let tick = 0;
  const inner: DockletAdapter = { ...createDisconnectedAdapter(), kind: "http", mappedOperations: new Set(["runStackCommand", "runContainerCommand"]),
    subscribeActions: (listener) => { push = listener; return stop; },
    runStackCommand: vi.fn(async () => "stack-run"), runContainerCommand: vi.fn(async (id) => `container-${id}`),
  };
  const gate = new RunGate(inner, () => tick);
  gate.setParentResolver((scope) => scope.startsWith("container:") ? `stack:${scope.slice(10).split("~")[0]}~parent` : undefined);
  return { gate, inner, stop, emit: (runs: CodedActionRun[]) => push(runs), advance: () => { tick += 600_001; } };
}
it("a known runtime ID absent from every list after ten seconds never becomes success or a replacement command", async () => {
  const f = fixture(); await f.gate.run("runStackCommand", ["edge~parent", "start"]); f.emit([]); f.advance();
  expect(await f.gate.run("runStackCommand", ["edge~parent", "start"])).toBe("stack-run");
  expect(isRunBusy(await f.gate.run("runStackCommand", ["edge~parent", "stop"]).catch((e: unknown) => e))).toBe(true);
  expect(f.inner.runStackCommand).toHaveBeenCalledTimes(1); expect(f.stop).not.toHaveBeenCalled();
});
it("ambiguous admission with unknown ID stays explicitly failed to confirm and does not auto-replace", async () => {
  const f = fixture(); vi.mocked(f.inner.runContainerCommand).mockRejectedValue(new AdapterOutcomeError({ kind: "error", code: "runtime.admission-unknown" }));
  await expect(f.gate.run("runContainerCommand", ["edge~child", "start"])).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  f.advance(); f.emit([]);
  await expect(f.gate.run("runContainerCommand", ["edge~child", "start"])).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  expect(isRunBusy(await f.gate.run("runStackCommand", ["edge~parent", "stop"]).catch((e: unknown) => e))).toBe(true);
  expect(f.inner.runContainerCommand).toHaveBeenCalledTimes(1); expect(f.inner.runStackCommand).not.toHaveBeenCalled();
});
it("parent-child exclusions hold in both directions while siblings and hosts stay independent", async () => {
  const f = fixture(); await f.gate.run("runContainerCommand", ["edge~a", "start"]);
  await f.gate.run("runContainerCommand", ["edge~b", "start"]);
  expect(isRunBusy(await f.gate.run("runStackCommand", ["edge~parent", "stop"]).catch((e: unknown) => e))).toBe(true);
  await f.gate.run("runStackCommand", ["direct~parent", "start"]);
  expect(isRunBusy(await f.gate.run("runContainerCommand", ["direct~a", "start"]).catch((e: unknown) => e))).toBe(true);
  expect(f.inner.runContainerCommand).toHaveBeenCalledTimes(2); expect(f.inner.runStackCommand).toHaveBeenCalledTimes(1);
});
it("existing unknown run blocks another operation and only an actual terminal disposition releases it", async () => {
  const f = fixture(); await f.gate.run("runStackCommand", ["edge~parent", "start"]);
  const run = mapRuntime(snapshot("edge", { id: "stack-run", resourceType: "project", resourceId: "parent" }), "edge");
  f.emit([run]); f.advance();
  expect(isRunBusy(await f.gate.run("runContainerCommand", ["edge~a", "start"]).catch((e: unknown) => e))).toBe(true);
  f.emit([{ ...run, status: "failed" }]); expect(f.gate.size).toBe(0);
  await f.gate.run("runContainerCommand", ["edge~a", "start"]); expect(f.inner.runContainerCommand).toHaveBeenCalledTimes(1);
});

it("after the retry budget a known missing run gets targeted reads only, deduplicated locally", async () => {
  const f = fixture(); f.inner.getRuntimeRun = vi.fn(async () => mapRuntime(snapshot("edge", { id: "stack-run" }), "edge"));
  await f.gate.run("runStackCommand", ["edge~parent", "start"]);
  f.emit([mapRuntime(snapshot("edge", { id: "stack-run" }), "edge")]); f.emit([]); f.advance();
  await f.gate.run("runStackCommand", ["edge~parent", "start"]);
  await f.gate.run("runStackCommand", ["edge~parent", "start"]);
  expect(f.inner.getRuntimeRun).toHaveBeenCalledExactlyOnceWith("edge", "stack-run"); expect(f.inner.runStackCommand).toHaveBeenCalledTimes(1);
});
