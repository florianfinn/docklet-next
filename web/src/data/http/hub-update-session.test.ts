// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original lifecycle decision-table, recovery and request-boundary regression coverage.

import { afterEach, expect, it, vi } from "vitest";
import type { components } from "@/api/generated/schema";
import type { CodedActionRun } from "../message-code";
import { runtimeError } from "./runtime-mapping";
import { createHubUpdateSession, HUB_UPDATE_RECORD } from "./hub-update-session";
import { createHttpAdapter } from "./http-adapter";
import { createSelfUpdates } from "./self-update";

type Operation = components["schemas"]["SystemSelfUpdateOperation"];
const accepted = { accepted: true, opId: "operation", runId: "update-run", phase: "reserved" as const };
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") } });
const envelope = (data: unknown) => json({ success: true, data });
const cleanups: (() => void)[] = [];
afterEach(() => { cleanups.splice(0).forEach(f => f()); sessionStorage.clear(); vi.useRealTimers(); vi.restoreAllMocks(); });
function fixture() {
  vi.useFakeTimers(); vi.setSystemTime(new Date("2026-11-19T12:00:00Z"));
  let op: Operation | undefined = { ...accepted, targetVersion: "1.0.1", helperRunning: true };
  const fetcher = vi.fn(async (path: RequestInfo | URL) => String(path).endsWith("/app-version") ? json({ currentVersion: "v1.0.1" }) : json({ version: "1.0.0", eligibility: { state: "checking", manual: null }, operation: op }));
  const readRun = vi.fn(async () => ({ id: "update-run", hostId: "0", status: "running" } as CodedActionRun));
  const session = createHubUpdateSession({ baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "example" }, publicFetch: fetcher as typeof fetch, readRun, origin: "https://hub.example.com", storage: sessionStorage });
  session.establish("admin"); cleanups.push(session.clear);
  const submit = vi.fn(async () => accepted);
  return { session, fetcher, readRun, submit, op: () => op!, setOp: (value: Operation | undefined) => { op = value; }, start: async () => { await session.start("1.0.1", submit); await vi.advanceTimersByTimeAsync(0); }, tick: async (ms = 3000) => { await vi.advanceTimersByTimeAsync(ms); } };
}
it("B2 version seen before health remains running; correlated healthy decides done", async () => {
  const f = fixture(); await f.start();
  expect(f.session.get().state).toMatchObject({ phase: "running", targetSeen: true });
  f.setOp({ ...f.op(), phase: "settled", outcome: "healthy" }); await f.tick();
  expect(f.session.get().state.phase).toBe("done");
  const count = f.fetcher.mock.calls.length; await f.tick(60_000); expect(f.fetcher).toHaveBeenCalledTimes(count);
});
it.each(["failed-health", "failed-pre-effect", "unknown"] as const)("B3 settled %s wins over a terminal run and target version", async outcome => {
  const f = fixture(); await f.start(); f.setOp({ ...f.op(), phase: "settled", outcome, code: "update.self-failed" });
  f.readRun.mockResolvedValue({ id: "update-run", hostId: "0", status: outcome === "unknown" ? "failed" : "succeeded" } as CodedActionRun); await f.tick();
  expect(f.session.get().state.phase).toBe(outcome === "unknown" ? "unknown" : "failed");
});
it("B3 foreign operation and run cannot complete our update", async () => {
  const f = fixture(); await f.start(); f.setOp({ ...f.op(), opId: "foreign", phase: "settled", outcome: "healthy" });
  f.readRun.mockResolvedValue({ id: "foreign", hostId: "0", status: "succeeded" } as CodedActionRun); await f.tick();
  expect(f.session.get().state.phase).toBe("running");
});
it("B2 terminal run waits two extra operation samples, then falls back", async () => {
  const f = fixture(); f.readRun.mockResolvedValue({ id: "update-run", hostId: "0", status: "succeeded" } as CodedActionRun);
  await f.start(); expect(f.session.get().state.phase).toBe("running"); await f.tick(); expect(f.session.get().state.phase).toBe("running");
  await f.tick(); expect(f.session.get().state.phase).toBe("done");
});
it("B3 unreadable operation and coded unknown failed run produce unknown", async () => {
  const f = fixture(); await f.start();
  f.fetcher.mockRejectedValue(new TypeError("offline"));
  f.readRun.mockResolvedValue({ id: "update-run", hostId: "0", status: "failed", error: { code: "images.errors.update_outcome-unknown", params: {} } } as CodedActionRun);
  await f.tick(); expect(f.session.get().state.phase).toBe("unknown");
});
it("B20 healthy operation is independent of a stalled version body", async () => {
  const f = fixture(); await f.start();
  f.setOp({ ...f.op(), phase: "settled", outcome: "healthy" });
  f.fetcher.mockImplementation(async path => String(path).endsWith("/app-version") ? new Response(new ReadableStream()) : json({ version: "1.0.1", eligibility: { state: "checking", manual: null }, operation: f.op() }));
  await f.tick(); expect(f.session.get().state.phase).toBe("done");
});
it("B20 slow operation has its own five-second timeout while public version arrives", async () => {
  const f = fixture(); await f.start();
  f.fetcher.mockImplementation(async path => String(path).endsWith("/app-version") ? json({ currentVersion: "v1.0.1" }) : new Response(new ReadableStream()));
  f.readRun.mockResolvedValue({ id: "update-run", hostId: "0", status: "succeeded" } as CodedActionRun);
  await f.tick(3000); expect(f.session.get().state.phase).toBe("running"); await f.tick(4999); expect(f.session.get().state.phase).toBe("running");
  await f.tick(1); expect(f.session.get().state.phase).toBe("done");
});
it("B8 late poll from a cleared operation cannot publish over the next operation", async () => {
  const f = fixture(); let release!: (value: Response) => void;
  f.fetcher.mockImplementationOnce(() => new Promise(resolve => { release = resolve; }));
  await f.start(); f.session.clear(); f.setOp({ ...f.op(), opId: "next", targetVersion: "1.0.2" });
  await f.session.start("1.0.2", async () => ({ ...accepted, opId: "next" }));
  release(json({ version: "1.0.1", eligibility: { state: "checking", manual: null }, operation: { ...f.op(), opId: "operation", phase: "settled", outcome: "healthy" } }));
  await f.tick(0); expect(f.session.get().state).toMatchObject({ phase: "running", opId: "next" });
});
it("B7 recovery adopts active operations and replays a recorded terminal operation once", async () => {
  const f = fixture(); await f.session.recover(); await f.tick(0);
  expect(f.session.get().state.phase).toBe("running"); const stored = sessionStorage.getItem(HUB_UPDATE_RECORD)!;
  f.session.clear(); f.setOp({ ...f.op(), phase: "settled", outcome: "healthy" });
  await f.session.recover(); expect(f.session.get().state.phase).toBe("idle");
  sessionStorage.setItem(HUB_UPDATE_RECORD, stored); await f.session.recover(); expect(f.session.get().state.phase).toBe("done");
  expect(sessionStorage.getItem(HUB_UPDATE_RECORD)).toBeNull(); f.session.clear(); await f.session.recover(); expect(f.session.get().state.phase).toBe("idle");
});
it("B19 recovery retries failed reads at five and ten seconds", async () => {
  const f = fixture(); f.fetcher.mockRejectedValueOnce(new TypeError("offline")).mockRejectedValueOnce(new TypeError("offline"));
  await f.session.recover(); await f.tick(4999); expect(f.fetcher).toHaveBeenCalledTimes(1);
  await f.tick(1); expect(f.fetcher).toHaveBeenCalledTimes(2); await f.tick(9999); expect(f.fetcher).toHaveBeenCalledTimes(2);
  await f.tick(1); expect(f.session.get().state.phase).toBe("running");
});
it("N-3 a rejected session stops recovery retries", async () => {
  const f = fixture(); f.fetcher.mockRejectedValueOnce(new TypeError("offline"));
  await f.session.recover(); expect(f.fetcher).toHaveBeenCalledTimes(1);
  f.session.unauthorized(); await f.tick(60_000);
  expect([f.fetcher.mock.calls.length, f.session.get().state.phase]).toEqual([1, "idle"]);
});
it.each(["{", JSON.stringify({}), JSON.stringify({ hubOrigin: "https://other.example.com", principalId: "admin", opId: "operation", startedAt: 0 }), JSON.stringify({ hubOrigin: "https://hub.example.com", principalId: "other", opId: "operation", startedAt: 0 }), JSON.stringify({ hubOrigin: "https://hub.example.com", principalId: "admin", opId: "operation", startedAt: 99999999999999 })])("B21 invalid stored record %s never replays a historical success", async stored => {
  const f = fixture(); f.setOp({ ...f.op(), phase: "settled", outcome: "healthy" }); sessionStorage.setItem(HUB_UPDATE_RECORD, stored);
  await f.session.recover(); expect(f.session.get().state.phase).toBe("idle"); expect(sessionStorage.getItem(HUB_UPDATE_RECORD)).toBeNull();
});
it("B21 valid record retains elapsed time and invalid active record starts now", async () => {
  const f = fixture(); const startedAt = Date.now() - 70_000;
  sessionStorage.setItem(HUB_UPDATE_RECORD, JSON.stringify({ hubOrigin: "https://hub.example.com", principalId: "admin", opId: "operation", startedAt }));
  await f.session.recover(); expect(f.session.get().state).toMatchObject({ phase: "running", startedAt });
  f.session.clear(); sessionStorage.setItem(HUB_UPDATE_RECORD, JSON.stringify({ hubOrigin: "https://hub.example.com", principalId: "admin", opId: "operation", startedAt: Date.now() - 3 * 60 * 60_000 }));
  await f.session.recover(); expect(f.session.get().state).toMatchObject({ phase: "running", startedAt: Date.now() });
});
it("B19 ambiguous intake adopts a new already-settled operation without resending", async () => {
  const f = fixture(); f.session.observe({ ...f.op(), opId: "baseline" }); f.setOp({ ...f.op(), phase: "settled", outcome: "healthy" });
  f.submit.mockRejectedValue(runtimeError("update.outcome-unknown"));
  await expect(f.session.start("1.0.1", f.submit)).resolves.toBe("update-run"); expect(f.submit).toHaveBeenCalledTimes(1); expect(f.session.get().state.phase).toBe("done");
});
it("B19 ambiguous intake rejects baseline and foreign targets for sixty seconds", async () => {
  const f = fixture(); f.session.observe(f.op()); f.submit.mockRejectedValue(runtimeError("update.outcome-unknown"));
  const outcome = f.session.start("1.0.1", f.submit).catch(e => e); await f.tick(30_000); expect(f.session.get().state.phase).toBe("starting");
  f.setOp({ ...f.op(), opId: "foreign", targetVersion: "2.0.0" }); await f.tick(30_000);
  expect(await outcome).toMatchObject({ outcome: { code: "update.outcome-unknown" } }); expect(f.session.get().state.phase).toBe("idle"); expect(f.submit).toHaveBeenCalledTimes(1);
});
it("B8 identity replacement cancels late accepted intake and public timers", async () => {
  const f = fixture(); let accept!: (value: typeof accepted) => void;
  const pending = f.session.start("1.0.1", () => new Promise(resolve => { accept = resolve; })).catch(e => e);
  expect(f.session.get().state.phase).toBe("starting"); f.session.establish("another-admin"); accept(accepted); await pending;
  await f.tick(60_000); expect(f.session.get().state.phase).toBe("idle"); expect(f.fetcher).not.toHaveBeenCalled();
});
it("B12 Direct and Edge agent followers leave the hub session idle", async () => {
  const f = fixture(); const api = createSelfUpdates({ baseUrl: "/api", client: { fetch: async () => json(accepted), token: async () => "example" }, capability: async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }), generation: () => 0, hubSession: f.session, readRun: async hostId => ({ id: "update-run", hostId, status: "succeeded" } as CodedActionRun) }); cleanups.push(api.reset);
  for (const host of ["direct", "edge"]) { await api.selfUpdate(host, { targetVersion: "1.0.1" }); await f.tick(1000); expect(f.session.get().state.phase).toBe("idle"); }
});
function httpFixture() {
  vi.useFakeTimers(); let outage: number | "network" | "body" | undefined, unauthorized: "operation" | "run" | undefined;
  let healthy = false, meUnauthorized = false, losePost = false, operationUnavailable = false, unknownRun = false;
  const source = { close() {}, onopen: null as ((event: Event) => void) | null, onerror: null as ((event: Event) => void) | null, onmessage: null as ((event: MessageEvent) => void) | null };
  let currentUser = "admin", csrf = "before".padEnd(43, "x"), renew = false;
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path.endsWith("/auth/mode")) return new Response(JSON.stringify({ success: true, data: { mode: "none" } }), { headers: { "X-CSRF-Token": csrf, "Content-Type": "application/json" } });
    if (path.endsWith("/auth/me")) return meUnauthorized ? json({}, 401) : envelope({ id: currentUser, username: "operator", requiresPasswordChange: false, isAdmin: true });
    if (path.endsWith("/auth/logout")) return envelope({});
    if (path.includes("/capabilities")) return envelope([{ operationId: "selfUpdate", resourceRef: { kind: "hub", id: "0" }, available: true }]);
    if (path.endsWith("/upgrade")) {
      if (losePost) throw new TypeError("lost response");
      if (renew && new Headers(init?.headers).get("X-CSRF-Token") !== csrf) return json({ code: "csrf.invalid" }, 403);
      return json(accepted, 202);
    }
    if (outage) { if (outage === "network") throw new TypeError("offline"); if (outage === "body") return new Response(new ReadableStream()); return new Response("<html>Gateway</html>", { status: outage }); }
    if (path.endsWith("/app-version")) return json({ currentVersion: "v1.0.1" });
    if (path.endsWith("/upgrade/check") && operationUnavailable) return json({}, 503);
    if (path.endsWith("/upgrade/check")) return unauthorized === "operation" ? json({}, 401) : json({ version: "1.0.0", eligibility: { state: "checking", manual: null }, operation: { ...accepted, targetVersion: "1.0.1", helperRunning: !healthy, ...(healthy ? { phase: "settled", outcome: "healthy" } : {}) } });
    if (path.endsWith("/activities/update-run")) return unauthorized === "run" ? json({}, 401) : envelope({ activity: { id: "update-run", environmentId: "0", type: "hub.update", status: unknownRun ? "failed" : "running", ...(unknownRun ? { messageCode: "update.outcome-unknown" } : {}), startedAt: "2026-11-19T12:00:00Z", resourceType: "system", resourceId: "0" }, messages: [], output: [], stepTrace: [], outputTruncated: false });
    return envelope([]);
  });
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => source });
  cleanups.push(() => adapter.hubUpdateSession!.clear());
  return { adapter, fetcher, source, unknownRun: () => { operationUnavailable = true; unknownRun = true; }, losePost: () => { losePost = true; }, loseMe: () => { meUnauthorized = true; }, healthy: () => { healthy = true; }, outage: (value: typeof outage) => { outage = value; }, unauthorized: (value: typeof unauthorized) => { unauthorized = value; }, replace: () => { currentUser = "replacement"; }, restart: () => { renew = true; csrf = "after".padEnd(43, "x"); } };
}
it.each([502, 503, 504, "network", "body"] as const)("B1 actual adapter survives %s outage and stalled body", async outage => {
  const f = httpFixture(); await f.adapter.getCurrentUser(); await f.adapter.selfUpdate("0", { targetVersion: "1.0.1" }); await vi.advanceTimersByTimeAsync(0);
  f.outage(outage); await vi.advanceTimersByTimeAsync(8000);
  expect(f.adapter.hubUpdateSession!.get().state).toMatchObject({ phase: "running", online: false });
});
it.each(["operation", "run"] as const)("B4 actual CSRF/activity %s 401 keeps signin with public version only", async source => {
  const f = httpFixture(); await f.adapter.getCurrentUser(); await f.adapter.selfUpdate("0", { targetVersion: "1.0.1" }); await vi.advanceTimersByTimeAsync(0);
  f.unauthorized(source); await vi.advanceTimersByTimeAsync(3000);
  expect(f.adapter.hubUpdateSession!.get().state).toMatchObject({ phase: "signin", versionSeen: "v1.0.1" });
  const count = f.fetcher.mock.calls.length; await vi.advanceTimersByTimeAsync(12_000);
  expect(f.fetcher.mock.calls.slice(count).every(([path]) => String(path).endsWith("/app-version"))).toBe(true);
});
it("B8 actual logout clears session before suspension; another principal clears it too", async () => {
  const f = httpFixture(); await f.adapter.getCurrentUser(); await f.adapter.selfUpdate("0", { targetVersion: "1.0.1" }); await vi.advanceTimersByTimeAsync(0);
  f.replace(); await f.adapter.getCurrentUser(); expect(f.adapter.hubUpdateSession!.get().state.phase).toBe("idle");
  await f.adapter.selfUpdate("0", { targetVersion: "1.0.1" }); await vi.advanceTimersByTimeAsync(0); await f.adapter.logout();
  const count = f.fetcher.mock.calls.length; await vi.advanceTimersByTimeAsync(60_000); expect(f.adapter.hubUpdateSession!.get().state.phase).toBe("idle"); expect(f.fetcher).toHaveBeenCalledTimes(count);
});
it("B22 same session survives restart and first write renews CSRF once", async () => {
  const f = httpFixture(); await f.adapter.getCurrentUser(); await f.adapter.selfUpdate("0", { targetVersion: "1.0.1" }); await vi.advanceTimersByTimeAsync(0);
  const modes = f.fetcher.mock.calls.filter(([path]) => String(path).endsWith("/auth/mode")).length;
  f.restart(); f.outage("network"); await vi.advanceTimersByTimeAsync(3000); f.outage(undefined); await vi.advanceTimersByTimeAsync(3000);
  expect(f.adapter.hubUpdateSession!.get().state.phase).toBe("running");
  // Ending observation permits a new explicit write; CSRF still holds the token minted before restart.
  f.adapter.hubUpdateSession!.clear(); await expect(f.adapter.selfUpdate("0", { targetVersion: "1.0.1" })).resolves.toBe("update-run");
  expect(f.fetcher.mock.calls.filter(([path]) => String(path).endsWith("/auth/mode"))).toHaveLength(modes + 1);
  expect(f.fetcher.mock.calls.filter(([path]) => String(path).endsWith("/upgrade"))).toHaveLength(3);
});

it("B1 repeated HTML outages release real admission slots and recover to healthy", async () => {
  const f = httpFixture(); await f.adapter.getCurrentUser(); await f.adapter.selfUpdate("0", { targetVersion: "1.0.1" }); await vi.advanceTimersByTimeAsync(0);
  f.outage(503); await vi.advanceTimersByTimeAsync(60_000);
  expect(f.adapter.hubUpdateSession!.get().state).toMatchObject({ phase: "running", online: false });
  f.outage(undefined); f.healthy(); await vi.advanceTimersByTimeAsync(3000);
  expect(f.adapter.hubUpdateSession!.get().state.phase).toBe("done");
});

it("B19 accepted POST delayed past settlement produces terminal view without resending", async () => {
  const f = fixture(); let accept!: (value: components["schemas"]["SystemSelfUpdateResponse"]) => void;
  const submit = vi.fn(() => new Promise<components["schemas"]["SystemSelfUpdateResponse"]>(resolve => { accept = resolve; }));
  const pending = f.session.start("1.0.1", submit);
  await f.tick(30_000); expect(f.session.get().state.phase).toBe("starting");
  f.setOp({ ...f.op(), phase: "settled", outcome: "healthy" }); accept({ ...accepted, phase: "settled" }); await pending; await f.tick(0);
  expect(f.session.get().state.phase).toBe("done"); expect(submit).toHaveBeenCalledTimes(1);
});
it("B21 a numeric-looking string is not a finite timestamp", async () => {
  const f = fixture(); f.setOp({ ...f.op(), phase: "settled", outcome: "healthy" });
  sessionStorage.setItem(HUB_UPDATE_RECORD, JSON.stringify({ hubOrigin: "https://hub.example.com", principalId: "admin", opId: "operation", startedAt: String(Date.now()) }));
  await f.session.recover(); expect(f.session.get().state.phase).toBe("idle"); expect(sessionStorage.getItem(HUB_UPDATE_RECORD)).toBeNull();
});

it("B4 actual SSE auth/me probe suspends identity into public-only signin", async () => {
  const f = httpFixture(); await f.adapter.getCurrentUser(); const off = f.adapter.subscribeChanges(() => {}); cleanups.push(off);
  await f.adapter.selfUpdate("0", { targetVersion: "1.0.1" }); await vi.advanceTimersByTimeAsync(0);
  f.loseMe(); f.source.onerror?.(new Event("error")); await vi.advanceTimersByTimeAsync(0);
  expect(f.adapter.hubUpdateSession!.get().state).toMatchObject({ phase: "signin", versionSeen: "v1.0.1" });
});

it("B21 current foreign-principal record never replays terminal operation", async () => {
  const f = fixture(); f.setOp({ ...f.op(), phase: "settled", outcome: "healthy" });
  sessionStorage.setItem(HUB_UPDATE_RECORD, JSON.stringify({ hubOrigin: "https://hub.example.com", principalId: "other", opId: "operation", startedAt: Date.now() }));
  await f.session.recover(); expect(f.session.get().state.phase).toBe("idle"); expect(sessionStorage.getItem(HUB_UPDATE_RECORD)).toBeNull();
});
it("B19 real HTTP lost intake response reconciles with one POST", async () => {
  const f = httpFixture(); await f.adapter.getCurrentUser(); f.losePost();
  await expect(f.adapter.selfUpdate("0", { targetVersion: "1.0.1" })).resolves.toBe("update-run");
  expect(f.adapter.hubUpdateSession!.get().state.phase).toBe("running");
  expect(f.fetcher.mock.calls.filter(([path]) => String(path).endsWith("/upgrade"))).toHaveLength(1);
});

it("B3 actual producer messageCode survives activity mapping for unknown fallback", async () => {
  const f = httpFixture(); await f.adapter.getCurrentUser(); await f.adapter.selfUpdate("0", { targetVersion: "1.0.1" }); await vi.advanceTimersByTimeAsync(0);
  f.unknownRun(); await vi.advanceTimersByTimeAsync(3000);
  expect(f.adapter.hubUpdateSession!.get().state.phase).toBe("unknown");
});

it("B3 settled observation without an outcome stays unreadable and running", async () => {
  const f = fixture(); await f.start(); f.setOp({ ...f.op(), phase: "settled", outcome: undefined }); await f.tick();
  expect(f.session.get().state.phase).toBe("running");
});

it("B7 stale administrator recovery subscription cannot read as a replacement simple user", async () => {
  const f = fixture(); f.session.establish("simple", false); await f.session.recover(); await f.tick(120_000);
  expect(f.fetcher).not.toHaveBeenCalled(); expect(f.session.get().state.phase).toBe("idle");
});
