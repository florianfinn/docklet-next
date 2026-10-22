// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { expect, it, vi } from "vitest";
import { createActivityReader } from "./activity-reader";
import { deferred } from "./activity-reader.test-support";
import { renderRun } from "../message-code";
import { runtimeState } from "@/features/activity/runtime-presentation";
import { envelope, response, RUN_ID, runtimeDetail, runtimeFixture, snapshot } from "./runtime-commands.test-support";

it.each([false, true])("aggregate reload partial=%s retains recorded loss, cancel knowledge and current intent", async (partial) => {
  const f = runtimeFixture(); f.state.run = snapshot("edge", { cancelRequested: true }); f.state.recovery.unknownReason = "transport";
  await f.adapter.runContainerCommand("edge~target", "start");
  await f.adapter.getRuntimeRun!("edge", RUN_ID);
  f.state.detailStatus = 503;
  f.state.activity = (url) => {
    if (!url.pathname.endsWith("/activities")) return envelope(runtimeDetail({ ...f.state.run, cancelRequested: false }));
    const result = response({ success: true, data: url.pathname.includes("/edge/") ? [f.state.run] : [], pagination: { totalItems: url.pathname.includes("/edge/") ? 1 : 0 } });
    if (partial && url.pathname.includes("/edge/")) result.headers.set("X-Docklet-Host-Reason", "agent.unavailable");
    return result;
  };
  const published = vi.fn(); const off = f.adapter.subscribeActions(published);
  try {
    const run = (await f.adapter.listActions()).find((row) => row.id === RUN_ID)!;
    expect(run).toMatchObject({ hostId: "edge", outcomeState: "unknown", cancelRequested: true, runtime: { unknownReason: "transport", canRelease: true, canCancel: true, currentIntent: true } });
    expect(runtimeState(renderRun(run, "en-US"))).toBe("unknown");
    expect(published.mock.lastCall?.[0].find((row: { id: string }) => row.id === RUN_ID)).toMatchObject({ runtime: { unknownReason: "transport", currentIntent: true } });
    expect(f.commands()).toHaveLength(1);
  } finally { off(); }
});
it.each(["load", "partial", "refreshOne"])("%s keeps released failed/unknown and late evidence when targeted detail fails", async (mode) => {
  const f = runtimeFixture(); f.state.run = snapshot("edge", { status: "failed", cancelRequested: true });
  f.state.recovery = { executorState: "terminal", unknownReason: "transport", disposition: "released", dispositionReason: "Executor stopped and inspected", releaseAcknowledged: true, executorResultCode: "runtime.failed" };
  await f.helper.getRuntimeRun("edge", RUN_ID);
  const reader = createActivityReader({ hosts: async () => ["edge"], page: async () => ({ items: [f.state.run], incomplete: mode === "partial" }), detail: async () => runtimeDetail(f.state.run), project: f.helper.project, retained: f.helper.retained, publish() {}, unauthorized() {} });
  await reader.load(); f.state.detailStatus = 503;
  if (mode === "refreshOne") await reader.refreshOne(RUN_ID, "edge"); else await reader.load();
  const run = reader.runs()[0]!;
  expect(run).toMatchObject({ status: "failed", outcomeState: "unknown", cancelRequested: true, runtime: { released: true, releaseReason: "Executor stopped and inspected", lateResult: "runtime.failed" } });
  expect(runtimeState(renderRun(run, "en-US"))).toBe("released"); expect(f.commands()).toHaveLength(0);
});
it("refreshOne retains the loss recovery form and a later authoritative answer replaces the retained observation", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge"); f.state.recovery.unknownReason = "transport";
  const reader = createActivityReader({ hosts: async () => ["edge"], page: async () => ({ items: [f.state.run], incomplete: false }), detail: async () => runtimeDetail(f.state.run), project: f.helper.project, retained: f.helper.retained, publish() {}, unauthorized() {} });
  await reader.load(); f.state.detailStatus = 503;
  await reader.refreshOne(RUN_ID, "edge");
  expect(reader.runs()[0]).toMatchObject({ runtime: { unknownReason: "transport", canRelease: true } });
  f.state.detailStatus = 200; f.state.run = snapshot("edge", { status: "success", outcomeState: "known" }); delete f.state.recovery.unknownReason;
  await reader.refreshOne(RUN_ID, "edge");
  expect(reader.runs()[0]).toMatchObject({ status: "succeeded", outcomeState: "known" }); expect(runtimeState(renderRun(reader.runs()[0]!, "en-US"))).toBeUndefined();
});
it.each(["failed", "cancelled"] as const)("absent detail for terminal%s is neutral, never pending or blocked", async (status) => {
  const f = runtimeFixture(); f.state.run = snapshot("edge", { status }); f.state.detailStatus = 503;
  const reader = createActivityReader({ hosts: async () => ["edge"], page: async () => ({ items: [f.state.run], incomplete: false }), detail: async () => runtimeDetail(f.state.run), project: f.helper.project, publish() {}, unauthorized() {} });
  await reader.load();
  expect(runtimeState(renderRun(reader.runs()[0]!, "en-US"))).toBe("unverified"); expect(f.commands()).toHaveLength(0);
});
it("fallback is scoped to host and reset identity even when a raw run ID collides", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge"); f.state.recovery.unknownReason = "transport";
  await f.helper.getRuntimeRun("edge", RUN_ID); f.state.detailStatus = 503;
  const reader = (host: string) => createActivityReader({ hosts: async () => [host], page: async () => ({ items: [snapshot(host)], incomplete: false }), detail: async () => runtimeDetail(snapshot(host)), project: f.helper.project, publish() {}, unauthorized() {} });
  const other = reader("direct"); await other.load();
  expect(other.runs()[0]).toMatchObject({ hostId: "direct" }); expect(other.runs()[0]).not.toHaveProperty("runtime.unknownReason");
  f.helper.reset(); const fresh = reader("edge"); await fresh.load(); expect(fresh.runs()[0]).not.toHaveProperty("runtime.unknownReason");
});
it("an older failed projection uses the newer authoritative targeted answer without overwriting it", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge"); f.state.recovery.unknownReason = "transport";
  await f.helper.getRuntimeRun("edge", RUN_ID);
  const reader = createActivityReader({ hosts: async () => ["edge"], page: async () => ({ items: [f.state.run], incomplete: false }), detail: async () => runtimeDetail(f.state.run), project: f.helper.project, retained: f.helper.retained, publish() {}, unauthorized() {} });
  const reached = deferred<void>(), release = deferred<void>(); let hold = true;
  f.state.read = async () => { if (hold) { hold = false; reached.resolve(); await release.promise; return response({}, 503); } return envelope(runtimeDetail(snapshot("edge", { status: "success", outcomeState: "known" }))); };
  const older = reader.load(); await reached.promise;
  await f.helper.getRuntimeRun("edge", RUN_ID); release.resolve(); await older;
  expect(reader.runs()[0]).toMatchObject({ status: "succeeded", outcomeState: "known" }); expect(f.helper.retained()[0]).toMatchObject({ status: "succeeded", outcomeState: "known" });
});
it("a late failed projection after reset cannot import recovery evidence into a new identity", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge"); f.state.recovery.unknownReason = "transport";
  await f.helper.getRuntimeRun("edge", RUN_ID);
  const reader = createActivityReader({ hosts: async () => ["edge"], page: async () => ({ items: [f.state.run], incomplete: false }), detail: async () => runtimeDetail(f.state.run), project: f.helper.project, retained: f.helper.retained, publish() {}, unauthorized() {} });
  const reached = deferred<void>(), release = deferred<void>();
  f.state.read = async () => { reached.resolve(); await release.promise; return response({}, 503); };
  const old = reader.load().catch((error: unknown) => error); await reached.promise; f.helper.reset(); reader.reset(); release.resolve(); await old;
  expect(reader.runs()).toEqual([]); expect(f.helper.retained()).toEqual([]);
});

it.each(["503", "malformed"])("newer generic settled observation wins through repeated %s targeted failures", async (fault) => {
  const f = runtimeFixture(); f.state.run = snapshot("edge"); f.state.recovery.unknownReason = "transport";
  await f.helper.runContainerCommand("edge~target", "start");
  await f.helper.getRuntimeRun("edge", RUN_ID);
  const reader = createActivityReader({ hosts: async () => ["edge"], page: async () => ({ items: [f.state.run], incomplete: false }), detail: async () => runtimeDetail(f.state.run), project: f.helper.project, retained: f.helper.retained, publish() {}, unauthorized() {} });
  f.state.read = () => fault === "503" ? response({}, 503) : envelope(runtimeDetail(f.state.run, { recovery: undefined }));
  f.state.run = snapshot("edge", { status: "success", outcomeState: "known" });
  for (let read = 0; read < 2; read++) {
    await reader.load();
    expect(reader.runs()[0]).toMatchObject({ status: "succeeded", outcomeState: "known", runtime: { currentIntent: true } });
    expect(reader.runs()[0]).not.toHaveProperty("runtime.unknownReason");
    expect(reader.runs()[0]).not.toHaveProperty("runtime.canRelease");
    expect(f.helper.retained()[0]).toMatchObject({ status: "succeeded", outcomeState: "known" });
  }
});
