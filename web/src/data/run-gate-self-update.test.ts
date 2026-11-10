// Source: docklet-next/issues/439
// Revision: 48f47c93878ecf1c27491d0214db80d5b38b547d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original self-update host admission and durable run regression tests.

import { expect, it, vi } from "vitest";
import { RunGate, GATE_RULES, isRunBusy } from "./run-gate";
import { createDisconnectedAdapter } from "./disconnected-adapter";
import type { CodedActionRun } from "./message-code";
import type { DockletAdapter } from "./adapter";

it("registers distinct self-update keys with the existing host exclusion scope", () => {
  expect(GATE_RULES.selfUpdate!(["0"])).toEqual({ key: "hub:self-update", scope: "hub" });
  expect(GATE_RULES.selfUpdate!(["example"])).toEqual({ key: "host:example:self-update", scope: "host:example" });
});
it("joins repeats, excludes host restart, and keeps a missing durable update reserved", async () => {
  let emit!: (runs: CodedActionRun[]) => void, now = 0;
  const inner: DockletAdapter = { ...createDisconnectedAdapter(), kind: "http", mappedOperations: new Set(["selfUpdate", "restartAgent"]),
    subscribeActions: cb => { emit = cb; return () => {}; }, selfUpdate: vi.fn(async () => "update-run"), restartAgent: vi.fn(async () => "restart-run") };
  const gate = new RunGate(inner, () => now);
  await gate.run("selfUpdate", ["example", { targetVersion: "1.0.1" }]); now = 20_000; emit([]);
  expect(await gate.run("selfUpdate", ["example", { targetVersion: "1.0.1" }])).toBe("update-run"); expect(inner.selfUpdate).toHaveBeenCalledTimes(1);
  expect(isRunBusy(await gate.run("restartAgent", ["example"]).catch((e: unknown) => e))).toBe(true);
  await gate.run("restartAgent", ["other"]); expect(inner.restartAgent).toHaveBeenCalledTimes(1);
  gate.reset();
});
it("a self-update discovered through ordinary activity prevents conflicting host work", async () => {
  let emit!: (runs: CodedActionRun[]) => void;
  const inner: DockletAdapter = { ...createDisconnectedAdapter(), kind: "http", mappedOperations: new Set(["selfUpdate", "restartAgent"]),
    subscribeActions: cb => { emit = cb; return () => {}; }, selfUpdate: vi.fn(async () => "hub-run"), restartAgent: vi.fn(async () => "restart-run") };
  const gate = new RunGate(inner);
  await gate.run("selfUpdate", ["0", { targetVersion: "1.0.1" }]);
  emit([{ id: "other-tab", sourceOperation: "agent.update", hostId: "example", kind: "activity", status: "running" } as CodedActionRun]);
  expect(isRunBusy(await gate.run("restartAgent", ["example"]).catch((e: unknown) => e))).toBe(true);
  emit([{ id: "other-tab", sourceOperation: "agent.update", hostId: "example", kind: "activity", status: "succeeded" } as CodedActionRun]);
  expect(await gate.run("restartAgent", ["example"])).toBe("restart-run"); gate.reset();
});
