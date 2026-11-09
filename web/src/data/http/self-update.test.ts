// Source: docklet-next/issues/439
// Revision: 48f47c93878ecf1c27491d0214db80d5b38b547d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original intake, problem-shape, capability and reload regression coverage.

import { afterEach, expect, it, vi } from "vitest";
import type { CodedActionRun } from "../message-code";
import type { ResourceRef } from "../capability";
import { createSelfUpdates } from "./self-update";
import { createHttpAdapter } from "./http-adapter";
import type { SelfUpdateInput } from "@/domain/self-update";

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
const accepted = { accepted: true, opId: "operation", phase: "reserved", runId: "update-run" };
const run = (status = "running", hostId = "0") => ({ id: "update-run", hostId, status } as CodedActionRun);
const cleanups: (() => void)[] = [];
afterEach(() => { cleanups.splice(0).forEach(f => f()); vi.useRealTimers(); });
function fixture() {
  vi.useFakeTimers(); let generation = 0;
  const fetcher = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => init?.method === "POST" ? json(accepted, 202) : json({ currentVersion: "1.0.0" }));
  const capability = vi.fn(async (operationId: string, resourceRef: ResourceRef) => ({ operationId, resourceRef, available: true }));
  const readRun = vi.fn(async () => run()); const reload = vi.fn();
  const api = createSelfUpdates({ baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "example" }, capability, generation: () => generation, readRun, reload });
  cleanups.push(api.reset);
  return { api, fetcher, capability, readRun, reload, identity: () => { generation++; api.reset(); } };
}
it("UI-b sends targetVersion only, discarding forged recovery fields", async () => {
  const f = fixture();
  await f.api.selfUpdate("0", { targetVersion: "1.0.1", targetReference: "example.com/forged:1.0.1", opId: "forged", hubVersion: "9.0.0" } as SelfUpdateInput);
  expect(f.capability).toHaveBeenCalledWith("selfUpdate", { kind: "hub", id: "0" });
  expect(JSON.parse(String(f.fetcher.mock.calls[0]![1]?.body))).toEqual({ targetVersion: "1.0.1" });
});
it.each([{ detail: "update.self-custom-image", status: 409 }, { code: "host.busy", status: 409 }, { code: "host.restarting", status: 409 }, { code: "host.unreachable", status: 503 }, { detail: "update.check-unavailable", status: 503 }])("retains actual Huma/host problem code $detail $code", async body => {
  const f = fixture(); f.fetcher.mockResolvedValue(json(body, body.status));
  await expect(f.api.selfUpdate("example", { targetVersion: "1.0.1" })).rejects.toMatchObject({ outcome: { code: "code" in body ? body.code : body.detail, status: body.status } });
  expect(f.readRun).not.toHaveBeenCalled();
});
it("checks plain DTOs and refuses malformed checks and false intake acknowledgements", async () => {
  const f = fixture(); const check = { version: "1.0.0", eligibility: { state: "not-eligible", code: "update.self-not-compose", manual: ["docker compose pull"] } };
  f.fetcher.mockResolvedValue(json(check)); expect(await f.api.getSelfUpdate("example")).toEqual(check);
  expect(String(f.fetcher.mock.calls[0]![0])).toBe("/api/environments/example/system/upgrade/check");
  f.fetcher.mockResolvedValue(json({ success: true, data: check })); await expect(f.api.getSelfUpdate("example")).rejects.toMatchObject({ outcome: { code: "update.check-unavailable" } });
  f.fetcher.mockResolvedValue(json({ ...accepted, accepted: false, code: "host.busy" }, 202)); await expect(f.api.selfUpdate("example", { targetVersion: "1.0.1" })).rejects.toMatchObject({ outcome: { code: "host.busy" } });
});
it("UI-c refuses administrator-only operations before check or start HTTP", async () => {
  const f = fixture(); f.capability.mockResolvedValue({ operationId: "selfUpdate", resourceRef: { kind: "host", id: "example" }, available: false });
  await expect(f.api.getSelfUpdate("example")).rejects.toMatchObject({ outcome: { kind: "not-available" } });
  await expect(f.api.selfUpdate("example", { targetVersion: "1.0.1" })).rejects.toMatchObject({ outcome: { kind: "not-available" } });
  expect(f.fetcher).not.toHaveBeenCalled();
});
it("A2-o survives delayed, offline and old-version reconnects, requiring success and target together exactly once", async () => {
  const f = fixture(); await f.api.selfUpdate("0", { targetVersion: "1.0.1" });
  let release!: (value: Response) => void;
  f.fetcher.mockImplementationOnce(() => new Promise(resolve => { release = resolve; }));
  await vi.advanceTimersByTimeAsync(1000); expect(f.readRun).toHaveBeenCalledTimes(1); expect(f.reload).not.toHaveBeenCalled();
  release(json({ currentVersion: "1.0.0" })); await vi.advanceTimersByTimeAsync(0);
  f.fetcher.mockRejectedValueOnce(new TypeError("offline")); await vi.advanceTimersByTimeAsync(1000); expect(f.reload).not.toHaveBeenCalled();
  // Reconnection with the old version is not evidence of an update.
  await vi.advanceTimersByTimeAsync(2000); expect(f.reload).not.toHaveBeenCalled();
  f.fetcher.mockResolvedValue(json({ currentVersion: "1.0.1" })); await vi.advanceTimersByTimeAsync(4000); expect(f.reload).not.toHaveBeenCalled();
  f.readRun.mockResolvedValue(run("succeeded")); f.fetcher.mockResolvedValue(json({ currentVersion: "1.0.0" }));
  await vi.advanceTimersByTimeAsync(8000); expect(f.reload).not.toHaveBeenCalled();
  f.fetcher.mockResolvedValue(json({ currentVersion: "1.0.1" })); await vi.advanceTimersByTimeAsync(10_000); expect(f.reload).toHaveBeenCalledTimes(1);
  await vi.advanceTimersByTimeAsync(30_000); expect(f.reload).toHaveBeenCalledTimes(1);
});
it.each(["failed", "cancelled"])("A2-o never reloads on %s even with the target version", async status => {
  const f = fixture(); await f.api.selfUpdate("0", { targetVersion: "1.0.1" }); f.readRun.mockResolvedValue(run(status)); f.fetcher.mockResolvedValue(json({ currentVersion: "1.0.1" }));
  await vi.advanceTimersByTimeAsync(60_000); expect(f.reload).not.toHaveBeenCalled(); expect(f.readRun).toHaveBeenCalledTimes(1);
});
it("never infers success from an unavailable or foreign run, and stops at identity change", async () => {
  const f = fixture(); await f.api.selfUpdate("0", { targetVersion: "1.0.1" }); f.fetcher.mockResolvedValue(json({ currentVersion: "1.0.1" }));
  f.readRun.mockRejectedValueOnce(new TypeError("offline")); await vi.advanceTimersByTimeAsync(1000); expect(f.reload).not.toHaveBeenCalled();
  f.readRun.mockResolvedValue(run("succeeded", "foreign")); await vi.advanceTimersByTimeAsync(1000); expect(f.reload).not.toHaveBeenCalled();
  f.identity(); await vi.advanceTimersByTimeAsync(60_000); expect(f.reload).not.toHaveBeenCalled(); expect(f.readRun).toHaveBeenCalledTimes(2);
});
it("follows agent activity without polling the app version and never repeats the POST", async () => {
  const f = fixture(); const first = f.api.selfUpdate("example", { targetVersion: "1.0.1" }); const second = f.api.selfUpdate("example", { targetVersion: "1.0.1" }); expect(first === second).toBe(true); await first;
  f.readRun.mockResolvedValue(run("succeeded", "example")); await vi.advanceTimersByTimeAsync(60_000);
  expect(f.readRun).toHaveBeenCalledTimes(1); expect(f.fetcher).toHaveBeenCalledTimes(1); expect(f.reload).not.toHaveBeenCalled();
});
it("registers exact descriptors in the actual HTTP adapter, including recovery hosts and a denied principal", async () => {
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname.endsWith("/capabilities")) return json({ success: true, data: [{ operationId: "selfUpdate", resourceRef: { kind: url.searchParams.get("kind"), id: url.searchParams.get("id") }, available: url.searchParams.get("id") !== "denied", disabledReasonCode: "access.admin-required" }] });
    return json({ version: "1.0.0", eligibility: { state: "not-eligible", code: "update.requires-hub-update", manual: ["docker compose pull"] } });
  });
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch });
  for (const ref of [{ kind: "hub", id: "0" }, { kind: "host", id: "update-required" }]) {
    expect((await adapter.getCapability("selfUpdate", ref)).available).toBe(true);
    expect((await adapter.getSelfUpdate(ref.id)).eligibility.code).toBe("update.requires-hub-update");
  }
  await expect(adapter.selfUpdate("denied", { targetVersion: "1.0.1" })).rejects.toMatchObject({ outcome: { reasonCode: "access.admin-required" } });
  expect(fetcher.mock.calls.some(([url]) => String(url).endsWith("/denied/system/upgrade"))).toBe(false);
});
it("uses the actual HTTP activity decoder for accepted updates and publishes the real run independently", async () => {
  vi.useFakeTimers(); let status = "running";
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path.endsWith("/capabilities")) return json({ success: true, data: [{ operationId: "selfUpdate", resourceRef: { kind: "host", id: "example" }, available: true }] });
    if (path.endsWith("/upgrade")) return json(accepted, 202);
    if (path.endsWith("/activities/update-run")) return json({ success: true, data: { activity: { id: "update-run", environmentId: "example", type: "agent.update", status, startedAt: "2026-11-09T00:00:00Z", resourceType: "system", resourceId: "example", metadata: { targetVersion: "1.0.1" } }, messages: [], output: [], stepTrace: [], outputTruncated: false } });
    throw new Error(`Unexpected request ${path}`);
  });
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch });
  await adapter.selfUpdate("example", { targetVersion: "1.0.1" });
  await vi.advanceTimersByTimeAsync(1000);
  // The activity read is the existing route and decoder, not a self-update status substitute.
  expect(fetcher.mock.calls.some(([url]) => String(url).includes("/environments/example/activities/update-run"))).toBe(true);
  status = "success"; await vi.advanceTimersByTimeAsync(1000);
  let observed: CodedActionRun[] = [];
  const off = adapter.subscribeActions(rows => { observed = rows; }); off();
  expect(observed.some(row => row.id === "update-run" && row.status === "succeeded" && row.sourceOperation === "agent.update")).toBe(true);
  expect(fetcher.mock.calls.filter(([url]) => String(url).endsWith("/upgrade"))).toHaveLength(1);
});
it("late polls from a previous identity cannot remove the next identity's follow ownership", async () => {
  const f = fixture(); let release!: (value: CodedActionRun) => void;
  f.readRun.mockImplementationOnce(() => new Promise(resolve => { release = resolve; }));
  await f.api.selfUpdate("0", { targetVersion: "1.0.1" }); await vi.advanceTimersByTimeAsync(1000);
  f.identity(); await f.api.selfUpdate("0", { targetVersion: "1.0.1" });
  release(run("succeeded")); await vi.advanceTimersByTimeAsync(0);
  f.fetcher.mockResolvedValueOnce(json({ version: "1.0.0", eligibility: { state: "checking", manual: null }, operation: { opId: "operation", runId: "update-run", targetVersion: "1.0.1", phase: "reserved", helperRunning: false } }));
  await f.api.getSelfUpdate("0");
  await vi.advanceTimersByTimeAsync(1000);
  expect(f.readRun).toHaveBeenCalledTimes(2); expect(f.reload).not.toHaveBeenCalled();
});
