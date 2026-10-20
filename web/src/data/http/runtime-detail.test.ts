// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { afterEach, expect, it, vi } from "vitest";
import { renderRun } from "../message-code";
import { runtimeState } from "@/features/activity/runtime-presentation";
import { deferred } from "./activity-reader.test-support";
import { envelope, response, RUN_ID, runtimeDetail, runtimeFixture, snapshot } from "./runtime-commands.test-support";

afterEach(() => vi.useRealTimers());
it.each(["0", "direct", "edge"])("%s retained hub detail has explicit knowledge without an outcome-loss warning for normal pending work", async (host) => {
  const f = runtimeFixture(); f.state.run = snapshot(host);
  const run = await f.adapter.getRuntimeRun!(host, RUN_ID);
  expect(runtimeState(renderRun(run, "en-US"))).toBe(host === "0" ? undefined : "unconfirmed");
  expect(run.cancelRequested).toBe(false);
  expect(f.calls.some(({ url }) => url.pathname.endsWith(`/environments/${host}/runtime-runs/${RUN_ID}`) && url.search === "?refresh=true")).toBe(true);
  expect(f.calls.some(({ url }) => url.pathname === "/api/environments")).toBe(false); expect(f.commands()).toHaveLength(0);
});
it("actual evidence loss is distinct from unconfirmed and later authoritative known response clears it", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge"); f.state.recovery.unknownReason = "transport";
  expect(runtimeState(renderRun(await f.adapter.getRuntimeRun!("edge", RUN_ID), "en-US"))).toBe("unknown");
  f.state.run = { ...f.state.run, outcomeState: "known", status: "success" }; delete f.state.recovery.unknownReason;
  const known = await f.adapter.getRuntimeRun!("edge", RUN_ID);
  expect(known.status).toBe("succeeded"); expect(runtimeState(renderRun(known, "en-US"))).toBeUndefined(); expect(f.commands()).toHaveLength(0);
});
it("unavailable detail leaves an admitted remote run unconfirmed; recovery never repeats its command", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge"); f.state.detailStatus = 503;
  await f.helper.runContainerCommand("edge~target", "start");
  await expect(f.helper.getRuntimeRun("edge", RUN_ID)).rejects.toBeTruthy();
  expect(f.published[0]).toMatchObject({ outcomeState: "unknown", cancelRequested: false });
  expect(runtimeState(renderRun(f.published[0]!, "en-US"))).toBe("unconfirmed");
  f.state.detailStatus = 200; f.state.run = { ...f.state.run, outcomeState: "known", status: "success" };
  expect((await f.helper.getRuntimeRun("edge", RUN_ID)).status).toBe("succeeded"); expect(f.commands()).toHaveLength(1);
});
it("offline host discovery cannot prevent targeted run links or prune an admitted active observation", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge"); const seen = vi.fn(); const off = f.adapter.subscribeActions(seen);
  try {
    await f.adapter.runContainerCommand("edge~target", "start");
    await f.adapter.getRuntimeRun!("edge", RUN_ID);
    expect((await f.adapter.listActions()).some((run) => run.id === RUN_ID)).toBe(true);
    expect(seen.mock.lastCall?.[0].some((run: { id: string }) => run.id === RUN_ID)).toBe(true);
    expect(f.commands()).toHaveLength(1);
  } finally { off(); }
});
it.each([false, true])("cancelRequested=%s is explicit, cancellation intent never means terminal cancellation", async (cancelRequested) => {
  const f = runtimeFixture(); f.state.run = snapshot("0", { cancelRequested });
  f.state.read = (_url, init) => init.method === "POST" ? envelope(f.state.run) : envelope(runtimeDetail(f.state.run));
  const run = await f.adapter.cancelRuntimeRun!("0", RUN_ID);
  expect(run).toMatchObject({ status: "running", cancelRequested }); expect(runtimeState(renderRun(run, "en-US"))).toBe(cancelRequested ? "cancelling" : undefined);
  f.state.run = { ...f.state.run, status: "cancelled", cancelRequested: true };
  expect((await f.adapter.getRuntimeRun!("0", RUN_ID)).status).toBe("cancelled"); expect(f.commands()).toHaveLength(0);
});
it.each(["no-admin", "no-read", "no-cancel", "no-reason", "no-ack"])("release refuses %s without a mutation request", async (fault) => {
  const f = runtimeFixture(); if (fault === "no-admin") f.state.admin = false;
  if (fault === "no-read") f.state.permissions = ["activities:cancel"];
  if (fault === "no-cancel") f.state.permissions = ["activities:read"];
  await expect(f.adapter.releaseRuntimeRun!("edge", RUN_ID, { reason: fault === "no-reason" ? "  " : "Executor stopped, Docker checked", acknowledgeRisk: (fault !== "no-ack") as true })).rejects.toBeTruthy();
  expect(f.calls.filter(({ init }) => init.method === "POST")).toHaveLength(0);
});
it("administrator release uses exact acknowledgment body and keeps failed/unknown plus late evidence", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge", { status: "failed", outcomeState: "unknown", messageCode: "runtime.interrupted" });
  f.state.recovery = { executorState: "terminal", releaseAcknowledged: true, disposition: "released", dispositionReason: "Inspected example state", executorResultCode: "runtime.failed" };
  f.state.read = (_url, init) => init.method === "POST" ? envelope(f.state.run) : envelope(runtimeDetail(f.state.run, { recovery: f.state.recovery }));
  const run = await f.adapter.releaseRuntimeRun!("edge", RUN_ID, { acknowledgeRisk: true, reason: "Inspected example state" });
  expect(run).toMatchObject({ status: "failed", outcomeState: "unknown" }); expect(runtimeState(renderRun(run, "en-US"))).toBe("released");
  expect(JSON.parse(f.calls.find(({ url }) => url.pathname.endsWith("/release"))!.init.body as string)).toEqual({ reason: "Inspected example state", acknowledged: true });
  const late = await f.adapter.getRuntimeRun!("edge", RUN_ID);
  expect(late).toMatchObject({ status: "failed", outcomeState: "unknown", runtime: { lateResult: "runtime.failed", released: true } }); expect(f.commands()).toHaveLength(0);
});
it("server remains definitive when permissions change after a readable grant", async () => {
  const f = runtimeFixture(); f.state.read = () => response({ detail: "Example denied" }, 403);
  await expect(f.adapter.cancelRuntimeRun!("0", RUN_ID)).rejects.toMatchObject({ outcome: { status: 403 } });
  expect(f.commands()).toHaveLength(0);
});
it("old generation detail cannot overwrite current observations and aborted requests are scoped", async () => {
  const f = runtimeFixture(); const reached = deferred<void>(), release = deferred<void>();
  f.state.read = async () => { reached.resolve(); await release.promise; return envelope(runtimeDetail(f.state.run)); };
  const result = f.helper.getRuntimeRun("0", RUN_ID).catch((error: unknown) => error);
  await reached.promise; f.helper.reset(); release.resolve(); await result;
  expect(f.published).toEqual([]); expect(f.calls.find(({ url }) => url.pathname.includes("runtime-runs"))!.init.signal?.aborted).toBe(true);
});
it("unconfirmed and unavailable detail retries are bounded; known cancellation refreshes until terminal and cleans timers", async () => {
  vi.useFakeTimers(); const f = runtimeFixture(); f.state.run = snapshot("edge");
  await f.helper.getRuntimeRun("edge", RUN_ID); f.helper.observe(true);
  await vi.advanceTimersByTimeAsync(20_000);
  expect(f.calls.filter(({ url }) => url.pathname.includes("runtime-runs"))).toHaveLength(4); expect(vi.getTimerCount()).toBe(0);
  f.state.run = snapshot("edge", { outcomeState: "known", cancelRequested: true });
  await f.helper.getRuntimeRun("edge", RUN_ID); await vi.advanceTimersByTimeAsync(4_000);
  expect(f.published.at(-1)).toMatchObject({ status: "running", cancelRequested: true });
  f.state.run.status = "cancelled"; await vi.advanceTimersByTimeAsync(4_000);
  expect(f.published.at(-1)?.status).toBe("cancelled"); expect(vi.getTimerCount()).toBe(0); expect(f.commands()).toHaveLength(0); f.helper.reset();
});

it("local queued remains known and interrupted completion is effect-unverified", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("0", { status: "queued" });
  const queued = await f.adapter.getRuntimeRun!("0", RUN_ID);
  expect(queued).toMatchObject({ status: "queued", outcomeState: "known", cancelRequested: false });
  expect(runtimeState(renderRun(queued, "en-US"))).toBeUndefined();
  f.state.run = { ...f.state.run, status: "failed", messageCode: "runtime.interrupted" };
  expect(runtimeState(renderRun(await f.adapter.getRuntimeRun!("0", RUN_ID), "en-US"))).toBe("interrupted");
  expect(f.commands()).toHaveLength(0);
});
it("older targeted detail cannot erase a later cancellation snapshot", async () => {
  const f = runtimeFixture();
  await f.helper.getRuntimeRun("0", RUN_ID);
  const reached = deferred<void>(), release = deferred<void>(); let hold = true;
  f.state.read = async (_url, init) => {
    if (init.method === "POST") return envelope({ ...f.state.run, cancelRequested: true });
    if (hold) { hold = false; reached.resolve(); await release.promise; return envelope(runtimeDetail(f.state.run)); }
    return envelope(runtimeDetail({ ...f.state.run, cancelRequested: true }));
  };
  const older = f.helper.detail("0", RUN_ID); await reached.promise;
  await f.helper.cancelRuntimeRun("0", RUN_ID); release.resolve(); await older;
  expect(f.published.at(-1)?.cancelRequested).toBe(true); expect(f.commands()).toHaveLength(0); f.helper.reset();
});

it("bounded terminal history does not exhaust admission capacity for a deliberate new intent", async () => {
  const f = runtimeFixture();
  for (let i = 0; i < 260; i++) {
    f.state.run = snapshot("0", { id: `retained-${i}`, status: "success" });
    await f.helper.getRuntimeRun("0", f.state.run.id);
  }
  expect(f.helper.retained()).toHaveLength(256); expect(f.commands()).toHaveLength(0);
  f.state.run = snapshot();
  expect(await f.helper.runContainerCommand("0~target", "start")).toBe(RUN_ID);
  expect(f.commands()).toHaveLength(1); expect(f.helper.retained()).toHaveLength(256); f.helper.reset();
});

it("malformed outcome objects are typed detail errors and cannot publish a run", async () => {
  const f = runtimeFixture();
  f.state.read = () => envelope(runtimeDetail({ ...f.state.run, outcomeState: { toString: null } as never }));
  await expect(f.helper.getRuntimeRun("0", RUN_ID)).rejects.toMatchObject({ outcome: { code: "runtime.invalid-response" } });
  expect(f.commands()).toHaveLength(0); expect(f.published).toEqual([]);
});
