// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original adapter-boundary proofs for hub maintenance holds, identity changes, native transports and host skips (#389, #391).

import { afterEach, expect, it, vi } from "vitest";

import { connectedFixture } from "@/production-connected.test-support";
import { AdapterOutcomeError } from "../capability";
import { hostReadDeadline } from "./host-read-deadline";
import { createDispatchGate } from "./dispatch-gate";
import { createHostReadPolicy } from "./host-read-policy";
import { createMaintenanceObserver } from "./maintenance-observer";
import { createHttpAdapter } from "./http-adapter";
import { vulnerabilityTransport } from "./vulnerability-transport";

const flush = async () => { for (let i = 0; i < 40; i++) await Promise.resolve(); };
const maintenanceAnswer = () => new Response(JSON.stringify({ title: "Service Unavailable", status: 503, detail: "backup.maintenance" }), { status: 503, headers: { "Content-Type": "application/problem+json" } });
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); });

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
  const m = { active: false, status: "ok" as "ok" | "down" | "revoked", revoked: false, hold: undefined as Promise<void> | undefined, hostStatus: undefined as string | undefined };
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
    if (m.hold) await m.hold;
    if (m.active) return maintenanceAnswer();
    if (m.revoked && url.pathname !== "/api/auth/mode") return new Response(JSON.stringify({ code: "auth.unauthorized" }), { status: 401 });
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
    // The first 401 ends the identity; the identity reset cancels the released reads, which are never retried.
    expect(results.every((error) => error instanceof Error)).toBe(true);
    expect(h.sent.filter((line) => line === "GET /api/backups/storage").length).toBeLessThanOrEqual(2);
    expect(unauthorized).toBeGreaterThanOrEqual(1);
    expect(h.adapter.maintenance!.state().active).toBe(false);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(h.sent.filter((line) => line === "GET /api/backups/storage").length).toBeLessThanOrEqual(2);
  } finally { window.removeEventListener("docklet:unauthorized", count); }
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

it("R1B-8 every backup adapter reads maintenance from code, nested data.code and a problem detail", async () => {
  for (const body of [{ code: "backup.maintenance" }, { success: false, data: { code: "backup.maintenance" } }, { status: 409, detail: "backup.maintenance" }]) {
    const h = hub();
    h.f.fetcher.mockImplementation(async (input: RequestInfo | URL) => String(input).endsWith("/backups/storage") ? Response.json(body, { status: 409 }) : Response.json({ active: false }));
    await expect(h.adapter.getHubBackupStorage()).rejects.toMatchObject({ outcome: { code: "backup.maintenance" } });
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
