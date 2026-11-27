// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original combined update web control and progress.

import { afterEach, expect, it, vi } from "vitest";
import { CancelledError } from "@tanstack/react-query";
import { createFleetUpdates } from "./fleet-update";
import { createHubUpdateSession } from "./hub-update-session";
import type { FleetStatus } from "@/domain/fleet-update";

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
const response = { fleetId: "fleet", runId: "parent", hubPhase: "update", hubOpId: "hub-operation" };
const active: FleetStatus = { active: true, fleetId: "fleet", runId: "parent", phase: "hub", targetVersion: "1.0.1", hub: { environmentId: "0", opId: "hub-operation", state: "reserved" }, agents: [] };
afterEach(() => vi.useRealTimers());
function fixture() {
  let generation = 0;
  const fetcher = vi.fn<typeof fetch>(async (_url, init) => init?.method === "POST" ? json(response, 202) : json(active));
  const session = createHubUpdateSession({ baseUrl: "/api", client: { fetch: fetcher, token: async () => "example" }, publicFetch: fetcher, readRun: vi.fn() });
  const start = vi.spyOn(session, "start").mockResolvedValue("");
  const api = createFleetUpdates({ baseUrl: "/api", client: { fetch: fetcher, token: async () => "example" }, generation: () => generation, capability: async () => ({ available: true }), hubSession: session });
  return { api, fetcher, start, identity: () => { ++generation; session.clear(); } };
}
it("starts the combined hub session with the admitted operation, no parent run as hub child", async () => {
  const f = fixture(); await f.api.updateEverything("1.0.1");
  expect(f.start).toHaveBeenCalledWith("1.0.1", expect.any(Function), { combined: true });
  expect(await f.start.mock.calls[0]![1]()).toEqual({ accepted: true, opId: "hub-operation", phase: "reserved" });
  expect(JSON.parse(String(f.fetcher.mock.calls[0]![1]?.body))).toEqual({ targetVersion: "1.0.1" });
});
it("keeps a current hub out of the full-screen session", async () => {
  const f = fixture(); f.fetcher.mockResolvedValue(json({ ...response, hubPhase: "current", hubOpId: undefined }, 202));
  await f.api.updateEverything("1.0.1"); expect(f.start).not.toHaveBeenCalled();
});
it.each([502, 503, 504, "network", "timeout"])("adopts after lost %s response with exactly one POST", async failure => {
  vi.useFakeTimers(); const f = fixture();
  f.fetcher.mockImplementation(async (_url, init) => {
    if (init?.method !== "POST") return json(active);
    if (failure === "network") throw new TypeError("offline");
    if (failure === "timeout") return new Promise(resolve => setTimeout(() => resolve(json(response, 202)), 30_000));
    return new Response("gateway", { status: failure as number });
  });
  const task = f.api.updateEverything("1.0.1");
  const duplicate = f.api.updateEverything("1.0.1"); expect(duplicate === task).toBe(true);
  if (failure === "timeout") await vi.advanceTimersByTimeAsync(15_000);
  await expect(task).resolves.toMatchObject({ fleetId: "fleet", hubOpId: "hub-operation" });
  expect(f.fetcher.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(1);
  expect(f.fetcher.mock.calls.filter(([, init]) => init?.method === "GET")).toHaveLength(1);
  expect(f.start).toHaveBeenCalledTimes(1);
});
it("does not adopt another target after a lost response", async () => {
  const f = fixture(); f.fetcher.mockImplementation(async (_url, init) => init?.method === "POST" ? new Response("", { status: 503 }) : json({ ...active, targetVersion: "1.0.2" }));
  await expect(f.api.updateEverything("1.0.1")).rejects.toMatchObject({ outcome: { status: 503 } }); expect(f.start).not.toHaveBeenCalled();
});
it.each(["code", "detail"])("retains refusal %s and does not reconcile or repeat it", async field => {
  const f = fixture(); f.fetcher.mockResolvedValue(json({ [field]: "update.all-busy" }, 409));
  await expect(f.api.updateEverything("1.0.1")).rejects.toMatchObject({ outcome: { code: "update.all-busy", status: 409 } });
  expect(f.fetcher).toHaveBeenCalledTimes(1);
});
it.each([
  { ...active, agents: [{}] }, { ...active, agents: [{ environmentId: "example", opId: "op", state: "future" }] },
  { ...active, hub: { ...active.hub, outcome: "future" } }, { ...active, hub: { ...active.hub, code: "future" } },
  { ...active, agents: [{ ...active.hub }] }, { ...active, phase: "future" }, { ...active, runId: "" },
])("rejects malformed status %#", async value => {
  const f = fixture(); f.fetcher.mockResolvedValue(json(value));
  await expect(f.api.getFleetUpdate()).rejects.toMatchObject({ outcome: { code: "http.invalid-response" } });
});
it("fences a delayed POST after identity replacement", async () => {
  const f = fixture(); let answer!: (value: Response) => void;
  f.fetcher.mockImplementation(() => new Promise(resolve => { answer = resolve; }));
  const task = f.api.updateEverything("1.0.1"); await Promise.resolve(); f.identity(); answer(json(response, 202));
  await expect(task).rejects.toBeInstanceOf(CancelledError); expect(f.start).not.toHaveBeenCalled();
});
it("ends the combined view with the plan's reason when the hub is skipped before any operation", async () => {
  const skipped: FleetStatus = { ...active, phase: "settled", outcome: "failed", active: false, hub: { environmentId: "0", opId: "hub-operation", state: "skipped", outcome: "skipped", code: "update.permission-denied", runId: "hub-skip" } };
  const fetcher = vi.fn<typeof fetch>(async (url, init) => {
    if (init?.method === "POST") return json(response, 202);
    if (String(url).endsWith("/system/upgrade/all")) return json(skipped);
    if (String(url).includes("/system/upgrade/check")) return json({ version: "1.0.0", eligibility: { state: "available", candidate: { version: "1.0.1" }, manual: [] } });
    return json({ currentVersion: "1.0.0" });
  });
  const session = createHubUpdateSession({ baseUrl: "/api", client: { fetch: fetcher, token: async () => "example" }, publicFetch: fetcher, readRun: vi.fn() });
  session.establish("operator");
  const api = createFleetUpdates({ baseUrl: "/api", client: { fetch: fetcher, token: async () => "example" }, generation: () => 0, capability: async () => ({ available: true }), hubSession: session });
  await api.updateEverything("1.0.1");
  await vi.waitFor(() => expect(session.get().state).toMatchObject({ phase: "failed", code: "update.permission-denied", runId: "hub-skip" }), { timeout: 8000 });
  session.clear();
});
it("keeps the hub's own refusal reason on a 503", async () => {
  const f = fixture(); f.fetcher.mockImplementation(async (_url, init) => init?.method === "POST" ? json({ code: "update.check-unavailable" }, 503) : json({ ...active, active: false }));
  await expect(f.api.updateEverything("1.0.1")).rejects.toMatchObject({ outcome: { code: "update.check-unavailable", status: 503 } });
  expect(f.fetcher.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(1);
});
