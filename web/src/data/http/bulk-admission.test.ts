// Source: docklet-next/issues/349
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original behavioural break-map proofs. M19 web consumer integration.

import { expect, it, vi } from "vitest";
import type { DockletAdapter } from "../adapter";
import { withRunGate, runGateOf } from "../run-gate";
import { runtimeFixture, envelope, response, RUN_ID } from "./runtime-commands.test-support";

it.each([{ status: 202, code: "run.started", outcome: "started" }, { status: 200, code: "run.joined", outcome: "joined" }] as const)("real runtime admission maps $code to $outcome", async ({ status, code, outcome }) => {
  const f = runtimeFixture();
  f.state.command = () => envelope({ code, activityId: RUN_ID, activity: f.state.run }, status);
  expect(await f.helper.admitRuntimeCommand({ kind: "container", id: "0~canonical-id", command: "start" })).toEqual({ outcome, runId: RUN_ID });
  expect(f.commands()).toHaveLength(1);
});
it("local join and busy are quiet for bulk, with single commands unchanged", async () => {
  const f = runtimeFixture(); let release!: () => void;
  const held = new Promise<void>((resolve) => { release = resolve; });
  f.state.command = async () => { await held; return envelope({ code: "run.started", activityId: RUN_ID, activity: f.state.run }, 202); };
  const raw = { kind: "demo", ...f.helper, subscribeActions: () => () => {}, subscribeChanges: () => () => {} } as unknown as DockletAdapter;
  const adapter = withRunGate(raw), notices = vi.fn(); runGateOf(adapter)!.subscribe(notices);
  const first = adapter.runContainerCommand("0~canonical-id", "start");
  const joined = adapter.admitRuntimeCommand!({ kind: "container", id: "0~canonical-id", command: "start" }, { quiet: true });
  await expect(adapter.admitRuntimeCommand!({ kind: "container", id: "0~canonical-id", command: "stop" }, { quiet: true })).rejects.toMatchObject({ code: "busy" });
  await vi.waitFor(() => expect(f.commands()).toHaveLength(1)); release();
  expect(await first).toBe(RUN_ID); expect(await joined).toEqual({ outcome: "joined", runId: RUN_ID }); expect(notices).toHaveBeenCalledTimes(0);
  await adapter.runContainerCommand("0~canonical-id", "start"); expect(notices).toHaveBeenCalledTimes(1);
});
it("real runtime refuses busy admission without manufacturing a started outcome", async () => {
  const f = runtimeFixture(); f.state.command = () => response({ success: false, data: { code: "run.busy", activeRunId: RUN_ID } }, 409);
  await expect(f.helper.admitRuntimeCommand({ kind: "container", id: "0~canonical-id", command: "start" })).rejects.toMatchObject({ outcome: { code: "run.busy" } });
});
