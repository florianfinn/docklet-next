// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original adapter-boundary proofs for hub maintenance holds, identity changes, native transports and host skips (#389, #391).

import { QueryObserver } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";

import { connectedFixture } from "@/production-connected.test-support";
import { createAppQueryClient } from "../app-query-client";
import { AdapterOutcomeError } from "../capability";
import { isMaintenanceError } from "../maintenance-retry";
import { isCancelledError } from "@tanstack/react-query";
import { identityLifetime } from "../identity-lifetime";
import { hostReadDeadline, inventoryReadBudget } from "./host-read-deadline";
import { createDispatchGate } from "./dispatch-gate";
import { createHostReadPolicy } from "./host-read-policy";
import { createMaintenanceObserver } from "./maintenance-observer";
import { createHttpAdapter } from "./http-adapter";
import { vulnerabilityTransport } from "./vulnerability-transport";

const flush = async () => { for (let i = 0; i < 40; i++) await Promise.resolve(); };
const maintenanceAnswer = () => new Response(JSON.stringify({ title: "Service Unavailable", status: 503, detail: "backup.maintenance" }), { status: 503, headers: { "Content-Type": "application/problem+json" } });
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); });
/** Records a promise's settlement so a test can assert it is still pending. */
const outcome = (promise: Promise<unknown>) => {
  const result: { settled?: unknown } = {};
  void promise.then((value) => { result.settled = value; }, (error: unknown) => { result.settled = error; });
  return result;
};

class FakeSource {
  readyState = 0;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  readonly url: string;
  constructor(url: string) { this.url = url; }
  close() { this.readyState = 2; }
  open() { this.readyState = 1; this.onopen?.({} as Event); }
  send(data: unknown, id = "1") { this.onmessage?.({ data: JSON.stringify(data), lastEventId: id } as MessageEvent); }
}

/** The connected fixture hub behind a maintenance gate whose status endpoint answers without session. */
function hub() {
  const f = connectedFixture(); f.state.signedIn = true;
  const original = f.fetcher.getMockImplementation()!;
  const m = { active: false, status: "ok" as "ok" | "down" | "revoked", revoked: false, hold: undefined as Promise<void> | undefined, holdPath: "/", hostStatus: undefined as string | undefined, ignoreAbort: false };
  const sent: string[] = [];
  f.fetcher.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    const method = (init?.method ?? "GET").toUpperCase();
    if (url.pathname === "/api/backups/maintenance") {
      if (m.status === "down") throw new TypeError("fetch failed");
      if (m.status === "revoked") return new Response(null, { status: 401 });
      return Response.json({ active: m.active, operation: "restore" });
    }
    sent.push(`${method} ${url.pathname}`);
    // Like the browser's fetch, a held request rejects when its caller aborts it, unless the sender ignores aborts.
    if (m.hold && url.pathname.startsWith(m.holdPath)) await Promise.race([m.hold, m.ignoreAbort ? m.hold : new Promise((_, reject) => {
      const signal = init?.signal;
      if (signal?.aborted) reject(signal.reason);
      signal?.addEventListener("abort", () => reject(signal.reason), { once: true });
    })]);
    // The hub answers sign-in and sign-out during maintenance; everything else meets the maintenance answer.
    if (m.active && !url.pathname.startsWith("/api/auth/")) return maintenanceAnswer();
    if (m.revoked && url.pathname !== "/api/auth/mode") return new Response(JSON.stringify({ code: "auth.unauthorized" }), { status: 401 });
    if (url.pathname === "/api/auth/logout") { f.state.signedIn = false; return Response.json({ success: true, data: null }); }
    if (url.pathname === "/api/backups/storage") return Response.json({ available: true, path: "/backups" });
    if (url.pathname === "/api/environments" && m.hostStatus) {
      // The hub's catalogue agrees with its environment stream.
      const answer = await original(input, init);
      const body = await answer.json() as { data: Array<Record<string, unknown>> };
      return Response.json({ ...body, data: body.data.map((row) => ({ ...row, status: m.hostStatus })) }, { headers: answer.headers });
    }
    return original(input, init);
  });
  const sources: FakeSource[] = [];
  const adapter = createHttpAdapter({ fetch: f.fetcher as typeof fetch, eventSource: (url: string) => { const source = new FakeSource(url); sources.push(source); return source as never; } });
  const statusReads = () => f.fetcher.mock.calls.filter(([input]) => String(input).endsWith("/backups/maintenance")).length;
  return { f, m, sent, sources, adapter, statusReads };
}

it("R1B-2 an active maintenance status read alone holds the next read and shows the state; inactive releases it", async () => {
  vi.useFakeTimers();
  const h = hub(); h.m.active = true;
  await h.adapter.getHubBackupMaintenance();
  expect(h.adapter.maintenance!.state()).toMatchObject({ active: true, operation: "restore" });
  const storage = h.adapter.getHubBackupStorage();
  await flush();
  expect(h.sent).not.toContain("GET /api/backups/storage");
  h.m.active = false;
  await vi.advanceTimersByTimeAsync(1_000);
  expect(await storage).toMatchObject({ available: true });
  expect(h.adapter.maintenance!.state().active).toBe(false);
});

it("R1B-3 a maintenance answer of a request cancelled by a sign-in does not start maintenance for the new identity", async () => {
  const h = hub();
  // The sender delivers the late answer even though the request was cancelled, so it reaches the gate's own check.
  h.m.ignoreAbort = true;
  // The CSRF bootstrap is complete, so the sign-in does not wait for the held read.
  await h.adapter.getAuthMode();
  let release!: () => void;
  h.m.hold = new Promise<void>((resolve) => { release = resolve; });
  const old = h.adapter.getHubBackupStorage().catch((error: unknown) => error);
  await flush();
  expect(h.sent).toContain("GET /api/backups/storage");
  h.m.hold = undefined;
  const login = h.adapter.login("operator", "example-password");
  await flush();
  h.m.active = true;
  release();
  await old;
  h.m.active = false;
  await login;
  await flush();
  expect(h.adapter.maintenance!.state().active).toBe(false);
  expect(h.statusReads()).toBe(0);
});

it("R1B-6 the native stream does not start during a hold and starts once maintenance ends", async () => {
  vi.useFakeTimers();
  const h = hub(); h.m.active = true;
  await h.adapter.getHubBackupMaintenance();
  const stop = h.adapter.subscribeChanges(() => {});
  await flush();
  expect(h.sources).toHaveLength(0);
  h.m.active = false;
  await vi.advanceTimersByTimeAsync(1_000);
  expect(h.sources).toHaveLength(1);
  stop();
});

it("R1B-6 a stream error during a hold closes the stream and reconnects only after the hold", async () => {
  vi.useFakeTimers();
  const h = hub();
  const stop = h.adapter.subscribeChanges(() => {});
  await flush();
  expect(h.sources).toHaveLength(1);
  h.sources[0]!.open();
  h.m.active = true;
  await h.adapter.getHubBackupMaintenance();
  h.sources[0]!.readyState = 0;
  h.sources[0]!.onerror?.({} as Event);
  await flush();
  expect(h.sources[0]!.readyState).toBe(2);
  expect(h.sources).toHaveLength(1);
  h.m.active = false;
  await vi.advanceTimersByTimeAsync(1_000);
  expect(h.sources).toHaveLength(2);
  stop();
});

it("R1B-6 a hub backup download waits for the hold to end", async () => {
  vi.useFakeTimers();
  const h = hub(); h.m.active = true;
  await h.adapter.getHubBackupMaintenance();
  const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  const download = h.adapter.downloadHubBackup("example");
  await flush();
  expect(click).not.toHaveBeenCalled();
  h.m.active = false;
  await vi.advanceTimersByTimeAsync(1_000);
  await download;
  expect(click).toHaveBeenCalledTimes(1);
});

it("R1B-5 the end of maintenance refreshes every topic once", async () => {
  vi.useFakeTimers();
  const h = hub();
  const topics: string[][] = [];
  const stop = h.adapter.subscribeChanges((next) => topics.push(next));
  h.m.active = true;
  await h.adapter.getHubBackupMaintenance();
  h.m.active = false;
  await vi.advanceTimersByTimeAsync(1_000);
  const full = topics.filter((list) => list.includes("security") && list.includes("settings") && list.includes("events"));
  expect(full).toHaveLength(1);
  stop();
});

it("R1B-9 restore restart: unreachable status, revoked session, released reads meet 401 once and end the identity", async () => {
  vi.useFakeTimers();
  const h = hub();
  let unauthorized = 0;
  const count = () => { unauthorized++; };
  window.addEventListener("docklet:unauthorized", count);
  let resets = 0;
  const stopListening = identityLifetime(h.adapter).listen(() => { resets++; });
  try {
    h.m.active = true;
    await h.adapter.getHubBackupMaintenance();
    const reads = [h.adapter.getHubBackupStorage().catch((error: unknown) => error), h.adapter.getHubBackupStorage().catch((error: unknown) => error)];
    await flush();
    h.m.status = "down";
    await vi.advanceTimersByTimeAsync(1_000);
    expect(h.adapter.maintenance!.state().active).toBe(true);
    // The restored hub is back: maintenance is over and the old session is revoked.
    h.m.active = false; h.m.revoked = true; h.m.status = "revoked";
    await vi.advanceTimersByTimeAsync(2_000);
    const results = await Promise.all(reads);
    // The first 401 ends the identity; its reset cancels the released reads, which are never retried.
    expect(results.map((error) => isCancelledError(error))).toEqual([true, true]);
    expect(h.sent.filter((line) => line === "GET /api/backups/storage").length).toBeLessThanOrEqual(2);
    expect([unauthorized > 0, resets]).toEqual([true, 1]);
    expect(h.adapter.maintenance!.state().active).toBe(false);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(h.sent.filter((line) => line === "GET /api/backups/storage").length).toBeLessThanOrEqual(2);
  } finally { window.removeEventListener("docklet:unauthorized", count); stopListening(); }
});

it("R1B-9 environment stream rows reach the read policy before change listeners run; online recovers with a read", async () => {
  const h = hub();
  const issued: Array<Promise<unknown>> = [];
  let armed = false;
  const stop = h.adapter.subscribeChanges((topics) => {
    if (armed && topics.includes("hosts")) issued.push(h.adapter.getBackupRepository("first", "background").catch((error: unknown) => error));
  });
  await flush();
  h.sources[0]!.open();
  const row = (status: string) => ({ channel: "environments", timestamp: "2026-11-11T12:00:00Z", environment: { environments: [{ id: "first", name: "first", status, enabled: true, apiUrl: "http://192.0.2.10:3553", isEdge: false }] } });
  await flush();
  const before = h.sent.filter((line) => line.endsWith("/environments/first/volumes/backups/repository")).length;
  expect(before).toBe(0);
  armed = true;
  h.m.hostStatus = "offline";
  h.sources[0]!.send(row("offline"), "1");
  await vi.waitFor(() => expect(issued.length).toBeGreaterThan(0));
  const skipped = await issued[0];
  expect(skipped).toBeInstanceOf(AdapterOutcomeError);
  expect(h.sent.some((line) => line.endsWith("/environments/first/volumes/backups/repository"))).toBe(false);
  h.m.hostStatus = "online";
  h.sources[0]!.send(row("online"), "2");
  await vi.waitFor(() => expect(issued.length).toBe(2));
  await issued[1];
  expect(h.sent.filter((line) => line.endsWith("/environments/first/volumes/backups/repository"))).toHaveLength(1);
  stop();
});

it("R1B-8 the hub backup and generic readers decode maintenance from code, nested data.code and a problem detail", async () => {
  for (const body of [{ code: "backup.maintenance" }, { success: false, data: { code: "backup.maintenance" } }, { status: 409, detail: "backup.maintenance" }]) {
    const h = hub();
    const base = h.f.fetcher.getMockImplementation()!;
    h.f.fetcher.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => /\/backups\/storage|\/api\/environments(\?|$)/.test(String(input)) ? Response.json(body, { status: 409 }) : base(input, init));
    await expect(h.adapter.getHubBackupStorage()).rejects.toMatchObject({ outcome: { code: "backup.maintenance" } });
    // The generic reader (capabilities, environments, activities, events) decodes the same shapes.
    await expect(h.adapter.listHosts()).rejects.toMatchObject({ outcome: { code: "backup.maintenance" } });
  }
});

it("R1B-1 a logical read deadline does not run out while a maintenance hold lasts longer than it", async () => {
  vi.useFakeTimers();
  const state = { active: true };
  const physical = vi.fn(async (input: RequestInfo | URL) => String(input).endsWith("/backups/maintenance") ? Response.json({ active: state.active }) : state.active ? maintenanceAnswer() : Response.json({ success: true, data: [] }));
  const observer = createMaintenanceObserver("/api/backups/maintenance", physical);
  const fetch = createDispatchGate(physical, "/api", observer, createHostReadPolicy());
  const read = hostReadDeadline(40_000, "activity.read-timeout")(async (signal) => (await fetch("/api/environments/d/containers", { signal })).json());
  const settled = read.then((value) => ({ value }), (error: unknown) => ({ error }));
  await vi.advanceTimersByTimeAsync(60_000);
  expect(observer.holding()).toBe(true);
  state.active = false;
  await vi.advanceTimersByTimeAsync(1_000);
  expect(await settled).toEqual({ value: { success: true, data: [] } });
  observer.reset();
});

it("R1B-4 a skipped security read decodes as an unavailable agent", async () => {
  const physical = vi.fn(async () => Response.json({ success: true, data: [] }));
  const policy = createHostReadPolicy();
  policy.applyStream([{ id: "d", status: "offline", enabled: true }]);
  const gate = createDispatchGate(physical, "/api", createMaintenanceObserver("/api/backups/maintenance", physical), policy);
  const request = vulnerabilityTransport("/api", { fetch: gate, token: async () => "synthetic" }, () => 0);
  await expect(request("/environments/d/vulnerabilities/runs", "GET", undefined, true, undefined, true, "background")).rejects.toMatchObject({ code: "agent.unavailable" });
  expect(physical).not.toHaveBeenCalled();
});

/** Fill the hold's 64 request slots with genuine held reads; a 65th read is answered with maintenance at once. */
async function fillHold(h: ReturnType<typeof hub>) {
  // The CSRF bootstrap is complete, so the reads themselves wait in the hold.
  await h.adapter.getAuthMode();
  h.m.active = true;
  await h.adapter.getHubBackupMaintenance();
  const reads = Array.from({ length: 64 }, () => h.adapter.getHubBackupStorage());
  await flush();
  await expect(h.adapter.getHubBackupStorage()).rejects.toMatchObject({ outcome: { code: "backup.maintenance" } });
  expect(h.sent.filter((line) => line === "GET /api/backups/storage")).toHaveLength(0);
  return reads;
}

it("R2B-2 with every hold slot taken the stream waits without a reconnect loop and starts once maintenance ends", async () => {
  vi.useFakeTimers();
  const h = hub();
  const reads = await fillHold(h);
  const stop = h.adapter.subscribeChanges(() => {});
  try {
    const then = vi.spyOn(Promise.prototype, "then");
    await flush();
    const calls = then.mock.calls.length;
    then.mockRestore();
    expect(calls).toBeLessThan(10);
    expect(h.sources).toHaveLength(0);
    h.m.active = false;
    await vi.advanceTimersByTimeAsync(1_000);
    expect(h.sources).toHaveLength(1);
    expect((await Promise.all(reads)).every((value) => value.available)).toBe(true);
  } finally { stop(); }
});

it("R2B-2 a stream stopped while it waits for a full hold does not start after the hold", async () => {
  vi.useFakeTimers();
  const h = hub();
  const reads = await fillHold(h);
  const stop = h.adapter.subscribeChanges(() => {});
  await flush();
  stop();
  h.m.active = false;
  await vi.advanceTimersByTimeAsync(1_000);
  await Promise.all(reads);
  expect(h.sources).toHaveLength(0);
});

it("R2B-3 a hub backup download with every hold slot taken does not navigate before the hold ends", async () => {
  vi.useFakeTimers();
  const h = hub();
  const reads = await fillHold(h);
  const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  const download = h.adapter.downloadHubBackup("example");
  await flush();
  await vi.advanceTimersByTimeAsync(500);
  expect(click).not.toHaveBeenCalled();
  h.m.active = false;
  await vi.advanceTimersByTimeAsync(1_000);
  await download;
  await Promise.all(reads);
  expect(click).toHaveBeenCalledTimes(1);
});

it("R2B-3 an identity change while a download waits for the hold cancels the download without navigation", async () => {
  vi.useFakeTimers();
  const h = hub();
  const reads = (await fillHold(h)).map((read) => read.catch((error: unknown) => error));
  const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  const download = h.adapter.downloadHubBackup("example").then(() => "navigated", (error: unknown) => error);
  await flush();
  // A sign-in replaces the identity without signing out, so only the download's own generation check stops it.
  await h.adapter.login("operator", "example-password");
  await vi.advanceTimersByTimeAsync(10);
  expect(isCancelledError(await download)).toBe(true);
  h.m.active = false;
  await vi.advanceTimersByTimeAsync(2_000);
  await Promise.all(reads);
  expect(click).not.toHaveBeenCalled();
});

/** A read through the real gate and observer that is held by maintenance, then stalls after the hold ends. */
function heldThenStalled() {
  const state = { active: true };
  const physical = vi.fn(async (input: RequestInfo | URL) => String(input).endsWith("/backups/maintenance") ? Response.json({ active: state.active }) : state.active ? maintenanceAnswer() : new Promise<Response>(() => {}));
  const observer = createMaintenanceObserver("/api/backups/maintenance", physical);
  const fetch = createDispatchGate(physical, "/api", observer, createHostReadPolicy());
  return { state, observer, read: (signal: AbortSignal) => fetch("/api/environments/d/containers", { signal }) };
}

it("R2B-1 a read still stalled after a 60 s hold spends only its remaining 40 s budget of active time", async () => {
  vi.useFakeTimers();
  const h = heldThenStalled();
  const result = outcome(hostReadDeadline(40_000, "activity.read-timeout")(h.read));
  await vi.advanceTimersByTimeAsync(60_000);
  expect(h.observer.holding()).toBe(true);
  h.state.active = false;
  // The status poll ends the hold at 61 s; 39 s of active time have passed at 100 s.
  await vi.advanceTimersByTimeAsync(40_000);
  expect(result.settled).toBeUndefined();
  await vi.advanceTimersByTimeAsync(2_000);
  expect(result.settled).toMatchObject({ outcome: { code: "activity.read-timeout" } });
  h.observer.reset();
});

it("R2B-1 an inventory budget taken before a 60 s hold leaves a host read started after it 30 s of active time", async () => {
  vi.useFakeTimers();
  const h = heldThenStalled();
  const budget = inventoryReadBudget(undefined, "http.error");
  // Another read meets maintenance and holds; the inventory's host read starts once the hold has ended (61 s).
  void h.read(new AbortController().signal).catch(() => undefined);
  await vi.advanceTimersByTimeAsync(60_000);
  h.state.active = false;
  await vi.advanceTimersByTimeAsync(1_000);
  const result = outcome(budget(h.read));
  await vi.advanceTimersByTimeAsync(29_000);
  expect(result.settled).toBeUndefined();
  await vi.advanceTimersByTimeAsync(2_000);
  expect(result.settled).toMatchObject({ outcome: { code: "http.error" } });
  h.observer.reset();
});

const envelope = (status: string, isEdge = false) => ({ channel: "environments", timestamp: "2026-11-11T12:00:00Z", environment: { environments: [{ id: "first", name: "first", status, enabled: true, apiUrl: "http://192.0.2.10:3553", isEdge }] } });
const repositoryReads = (h: ReturnType<typeof hub>) => h.sent.filter((line) => line.endsWith("/environments/first/volumes/backups/repository")).length;

it("R2-g an Edge host going offline on the stream skips a tagged read still waiting in host admission", async () => {
  const h = hub();
  const stop = h.adapter.subscribeChanges(() => {});
  await flush();
  h.sources[0]!.open();
  h.sources[0]!.send(envelope("online", true), "1");
  await flush();
  let release!: () => void;
  h.m.hold = new Promise<void>((resolve) => { release = resolve; }); h.m.holdPath = "/api/environments/first/";
  const busy = Array.from({ length: 5 }, () => h.adapter.getBackupRepository("first", "poll").catch((error: unknown) => error));
  await flush(); await flush();
  expect(repositoryReads(h)).toBe(5);
  const queued = h.adapter.getBackupRepository("first", "background").catch((error: unknown) => error);
  const queuedResult = outcome(queued);
  await flush();
  expect(repositoryReads(h)).toBe(5);
  h.m.hostStatus = "offline";
  h.sources[0]!.send(envelope("offline", true), "2");
  await flush();
  release(); h.m.hold = undefined;
  // The stream's host change discards the five answers; their slots are released, so the queued read proceeds.
  expect((await Promise.all(busy)).every((error) => isCancelledError(error))).toBe(true);
  // The queued read is stale after the host change and is never dispatched; a new read is skipped at the gate.
  await vi.waitFor(() => expect(queuedResult.settled).toBeDefined(), { timeout: 1_000 });
  expect(isCancelledError(queuedResult.settled)).toBe(true);
  expect(repositoryReads(h)).toBe(5);
  expect(await h.adapter.getBackupRepository("first", "background").catch((error: unknown) => error)).toMatchObject({ outcome: { code: "agent.unavailable" } });
  expect(repositoryReads(h)).toBe(5);
  stop();
});

it("R2-h a host in standby skips poll reads but sends background reads; back online, poll reads are sent again", async () => {
  const h = hub();
  const stop = h.adapter.subscribeChanges(() => {});
  await flush();
  h.sources[0]!.open();
  h.m.hostStatus = "standby";
  h.sources[0]!.send(envelope("standby"), "1");
  await flush();
  expect(await h.adapter.getBackupRepository("first", "poll").catch((error: unknown) => error)).toMatchObject({ outcome: { code: "agent.unavailable" } });
  expect(repositoryReads(h)).toBe(0);
  await h.adapter.getBackupRepository("first", "background").catch(() => undefined);
  expect(repositoryReads(h)).toBe(1);
  h.m.hostStatus = "online";
  h.sources[0]!.send(envelope("online"), "2");
  await flush();
  await h.adapter.getBackupRepository("first", "poll").catch(() => undefined);
  expect(repositoryReads(h)).toBe(2);
  stop();
});

it("R1B-5 a warm hosts query whose capability lookup overflows a full hold keeps its cards and success state, then refreshes", async () => {
  vi.useFakeTimers();
  const h = hub();
  const client = createAppQueryClient();
  const observer = new QueryObserver(client, { queryKey: ["hosts"], queryFn: () => h.adapter.listHosts(), staleTime: 0 });
  const states: Array<{ status: string; cards: number }> = [];
  const stopObserver = observer.subscribe((result) => states.push({ status: result.status, cards: result.data?.length ?? 0 }));
  const stop = h.adapter.subscribeChanges(() => {});
  await vi.advanceTimersByTimeAsync(0);
  const warm = observer.getCurrentResult();
  expect([warm.status, (warm.data?.length ?? 0) > 0]).toEqual(["success", true]);
  // The environment stream invalidates capabilities, so the refetch must look them up again.
  h.sources[0]!.open(); h.sources[0]!.send(envelope("online"));
  const reads = (await fillHold(h)).map((read) => read.catch(() => undefined));
  states.length = 0;
  void observer.refetch();
  await vi.advanceTimersByTimeAsync(5_000);
  expect(states.some((state) => state.status === "error" || state.cards === 0)).toBe(false);
  h.m.active = false;
  await vi.advanceTimersByTimeAsync(10_000);
  await Promise.all(reads);
  expect([observer.getCurrentResult().status, observer.getCurrentResult().fetchStatus]).toEqual(["success", "idle"]);
  stop(); stopObserver();
});

it("R1B-5 the capability lookup itself rejects an overflowing read with the maintenance outcome, not capability.unavailable", async () => {
  vi.useFakeTimers();
  const h = hub();
  await h.adapter.listHosts();
  const stop = h.adapter.subscribeChanges(() => {});
  await vi.advanceTimersByTimeAsync(0);
  h.sources[0]!.open(); h.sources[0]!.send(envelope("online"));
  const reads = (await fillHold(h)).map((read) => read.catch(() => undefined));
  const lookup = await h.adapter.getCapability("listHosts").then((value) => value, (error: unknown) => error);
  expect(isMaintenanceError(lookup)).toBe(true);
  h.m.active = false;
  await vi.advanceTimersByTimeAsync(2_000);
  await Promise.all(reads);
  stop();
});

/** The hub fixture with the settings operations available, so their reads reach the hold. */
function settingsHub() {
  const h = hub();
  const original = h.f.fetcher.getMockImplementation()!;
  h.f.fetcher.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const answer = await original(input, init);
    if (new URL(String(input), "https://hub.example.com").pathname !== "/api/capabilities" || !answer.ok) return answer;
    const body = await answer.json() as { data: Array<{ operationId: string; resourceRef: unknown }> };
    const extra = ["getScanSettings", "saveSettings"].map((operationId) => ({ operationId, available: true, resourceRef: body.data[0]!.resourceRef }));
    return Response.json({ ...body, data: [...body.data, ...extra] }, { headers: answer.headers });
  });
  return h;
}

it.each([
  ["volume", async (h: ReturnType<typeof hub>) => { const [volume] = await h.adapter.listVolumes(); return () => h.adapter.getVolume(volume!.id); }],
  ["scan settings", async (h: ReturnType<typeof hub>) => { await h.adapter.getCapability("getScanSettings"); return () => h.adapter.getScanSettings!(); }],
  ["update settings", async (h: ReturnType<typeof hub>) => { await h.adapter.getCapability("saveSettings"); return () => h.adapter.getUpdateSettings!(); }],
  ["connected settings", async (h: ReturnType<typeof hub>) => () => h.adapter.getConnectedSettings!("installation")],
])("B16/B21/B22 a %s read refused by a full hold rejects with an error the maintenance retry policy recognises", async (_family, prepare) => {
  vi.useFakeTimers();
  const h = settingsHub();
  const read = await prepare(h);
  const reads = (await fillHold(h)).map((pending) => pending.catch(() => undefined));
  const refused = await read().then(() => "resolved", (error: unknown) => error);
  expect(isMaintenanceError(refused)).toBe(true);
  h.m.active = false;
  await vi.advanceTimersByTimeAsync(2_000);
  await Promise.all(reads);
});
