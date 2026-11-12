// @vitest-environment node
// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original maintenance hold, replay, waiter bound and host skip proofs at physical dispatch (#389, #391).

import { afterEach, expect, it, vi } from "vitest";
import { createDispatchGate } from "./dispatch-gate";
import { createHostReadAdmission } from "./host-read-admission";
import { createHostReadPolicy, READ_TAG_HEADER } from "./host-read-policy";
import { createMaintenanceObserver, MAINTENANCE_MAX_HOLD_MS, MAINTENANCE_WAITERS } from "./maintenance-observer";

const flush = async () => { for (let i = 0; i < 30; i++) await Promise.resolve(); };
const maintenanceAnswer = () => new Response(JSON.stringify({ title: "Service Unavailable", status: 503, detail: "backup.maintenance" }), { status: 503, headers: { "Content-Type": "application/problem+json" } });
afterEach(() => { vi.useRealTimers(); });

/** A hub whose maintenance gate refuses every request except its status endpoint. */
function hub() {
  const state = { active: false, status: "ok" as "ok" | "down" | "revoked" };
  const sent: string[] = [];
  const physical = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    const method = (init?.method ?? "GET").toUpperCase();
    if (url.pathname === "/api/backups/maintenance") {
      if (state.status === "down") throw new TypeError("fetch failed");
      if (state.status === "revoked") return new Response(null, { status: 401 });
      return Response.json({ active: state.active, operation: "backup" });
    }
    sent.push(`${method} ${url.pathname}`);
    return state.active ? maintenanceAnswer() : Response.json({ success: true, data: {} });
  });
  const observer = createMaintenanceObserver("/api/backups/maintenance", physical);
  const policy = createHostReadPolicy();
  const fetch = createDispatchGate(physical, "/api", observer, policy);
  /** Start a read that meets the maintenance answer; it stays held, so it is not awaited. */
  const meet = (path = "/api/environments/d/images") => { void fetch(path).catch(() => undefined); return flush(); };
  return { state, sent, physical, observer, policy, fetch, meet };
}

it("R2-e a read that meets maintenance is held and sent again once it ends", async () => {
  vi.useFakeTimers();
  const h = hub(); h.state.active = true;
  const read = h.fetch("/api/environments/d/containers");
  await flush();
  expect(h.observer.state().active).toBe(true);
  expect(h.sent).toEqual(["GET /api/environments/d/containers"]);
  h.state.active = false;
  await vi.advanceTimersByTimeAsync(1_000);
  expect((await read).status).toBe(200);
  expect(h.sent).toEqual(["GET /api/environments/d/containers", "GET /api/environments/d/containers"]);
  expect(h.observer.state().active).toBe(false);
});

it("R2-e reads started after the signal wait without dispatch; a read-only projection is held, a write is sent once", async () => {
  vi.useFakeTimers();
  const h = hub(); h.state.active = true;
  await h.meet();
  const held = h.fetch("/api/environments/d/stacks");
  const projection = h.fetch("/api/environments/d/images/vulnerabilities/summaries", { method: "POST", body: JSON.stringify({ imageIds: ["a"] }) });
  const write = h.fetch("/api/environments/d/containers/c1/start", { method: "POST" });
  await flush();
  expect(h.sent).toEqual(["GET /api/environments/d/images", "POST /api/environments/d/containers/c1/start"]);
  expect((await write).status).toBe(503);
  h.state.active = false;
  await vi.advanceTimersByTimeAsync(1_000);
  expect((await held).status).toBe(200);
  expect((await projection).status).toBe(200);
  expect(h.sent.filter((line) => line.endsWith("/start"))).toHaveLength(1);
  expect(h.sent).toContain("POST /api/environments/d/images/vulnerabilities/summaries");
});

it("R2-e a held read answered with maintenance again is retried at most twice", async () => {
  const physical = vi.fn(async (input: RequestInfo | URL) => String(input).endsWith("/backups/maintenance") ? Response.json({ active: false }) : maintenanceAnswer());
  const observer = createMaintenanceObserver("/api/backups/maintenance", physical);
  const fetch = createDispatchGate(physical, "/api", observer, createHostReadPolicy());
  const response = await fetch("/api/environments/d/containers");
  expect(response.status).toBe(503);
  expect(physical.mock.calls.filter(([input]) => String(input).endsWith("/containers"))).toHaveLength(3);
});

it("R2-e an aborted waiter rejects at once and is never dispatched", async () => {
  vi.useFakeTimers();
  const h = hub(); h.state.active = true;
  await h.meet();
  const controller = new AbortController();
  const waiting = h.fetch("/api/environments/d/stacks", { signal: controller.signal });
  await flush();
  expect(h.observer.waiting()).toBe(2);
  controller.abort();
  await expect(waiting).rejects.toBeDefined();
  expect(h.observer.waiting()).toBe(1);
  h.state.active = false;
  await vi.advanceTimersByTimeAsync(1_000);
  expect(h.sent).not.toContain("GET /api/environments/d/stacks");
});

it("R2-e the waiter bound settles further held requests with the maintenance answer without dispatch", async () => {
  vi.useFakeTimers();
  const h = hub(); h.state.active = true;
  await h.meet();
  // The read that met maintenance is the first waiter.
  const waiting = Array.from({ length: MAINTENANCE_WAITERS - 1 }, (_, i) => h.fetch(`/api/environments/d/volumes/${i}`));
  await flush();
  expect(h.observer.waiting()).toBe(MAINTENANCE_WAITERS);
  const overflow = h.fetch("/api/environments/d/networks");
  await flush();
  expect(h.observer.waiting()).toBe(MAINTENANCE_WAITERS);
  expect((await overflow).status).toBe(503);
  expect(h.sent).not.toContain("GET /api/environments/d/networks");
  h.state.active = false;
  await vi.advanceTimersByTimeAsync(1_000);
  expect((await Promise.all(waiting)).every((response) => response.status === 200)).toBe(true);
});

it("R2-f a hold ends after its maximum and marks the maintenance overdue; requests then proceed", async () => {
  vi.useFakeTimers();
  const h = hub(); h.state.active = true;
  await h.meet();
  const held = h.fetch("/api/environments/d/stacks");
  await vi.advanceTimersByTimeAsync(MAINTENANCE_MAX_HOLD_MS - 1_000);
  expect(h.sent).not.toContain("GET /api/environments/d/stacks");
  await vi.advanceTimersByTimeAsync(1_000);
  expect(h.observer.state()).toEqual({ active: true, operation: "backup", overdue: true });
  expect(h.sent).toContain("GET /api/environments/d/stacks");
  expect((await held).status).toBe(503);
  // An overdue hold does not replay: the request surfaces its own answer.
  expect(h.sent.filter((line) => line.endsWith("/stacks"))).toHaveLength(1);
});

it("R2-f an unreachable status endpoint backs off 1, 2, 4, 5 s and keeps the hold", async () => {
  vi.useFakeTimers();
  const h = hub(); h.state.active = true; h.state.status = "down";
  await h.meet();
  const statusReads = () => h.physical.mock.calls.filter(([input]) => String(input).endsWith("/backups/maintenance")).length;
  await flush();
  expect(statusReads()).toBe(1);
  for (const [delay, total] of [[1_000, 2], [2_000, 3], [4_000, 4], [5_000, 5], [5_000, 6]] as const) {
    await vi.advanceTimersByTimeAsync(delay - 1);
    expect(statusReads()).toBe(total - 1);
    await vi.advanceTimersByTimeAsync(1);
    expect(statusReads()).toBe(total);
  }
  expect(h.observer.holding()).toBe(true);
});

it("R2-e restore restart: unreachable status, revoked session, release, then identity reset rejects the rest", async () => {
  vi.useFakeTimers();
  const h = hub(); h.state.active = true;
  await h.meet();
  const first = h.fetch("/api/environments/d/stacks");
  h.state.status = "down";
  await vi.advanceTimersByTimeAsync(1_000);
  expect(h.observer.holding()).toBe(true);
  // The hub is back with the restored database: the old session is revoked, maintenance is over.
  h.state.active = false; h.state.status = "revoked";
  await vi.advanceTimersByTimeAsync(2_000);
  expect((await first).status).toBe(200);
  expect(h.observer.state().active).toBe(false);
  // A later maintenance hold met by identity loss: the reset rejects its waiter, which is never sent.
  h.state.active = true; h.state.status = "ok";
  await h.meet("/api/environments/d/volumes");
  const late = h.fetch("/api/environments/d/networks");
  await flush();
  h.observer.reset();
  await flush();
  expect(h.sent).not.toContain("GET /api/environments/d/networks");
  await expect(late).rejects.toBeDefined();
  h.state.active = false;
  await vi.advanceTimersByTimeAsync(5_000);
  expect(h.sent).not.toContain("GET /api/environments/d/networks");
});

it("R2-g a tagged read queued in admission is skipped at dispatch once the stream reports the host offline", async () => {
  const sent: string[] = [];
  const gates: Array<() => void> = [];
  const physical = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
    sent.push(new URL(String(input), "https://hub.example.com").pathname);
    await new Promise<void>((resolve) => gates.push(resolve));
    return Response.json({ success: true, data: [] });
  });
  const observer = createMaintenanceObserver("/api/backups/maintenance", physical);
  const policy = createHostReadPolicy();
  policy.applyStream([{ id: "d", status: "online", enabled: true }, { id: "e", status: "online", enabled: true }]);
  const fetch = createHostReadAdmission(createDispatchGate(physical, "/api", observer, policy), "/api", () => "http/1.1");
  const busy = Array.from({ length: 5 }, (_, i) => fetch(`/api/environments/d/containers/${i}`));
  const tagged = fetch("/api/environments/d/volumes/backups/overview", { headers: { [READ_TAG_HEADER]: "background" } });
  const gesture = fetch("/api/environments/d/containers/detail");
  await flush();
  expect(sent).toHaveLength(5);
  policy.applyStream([{ id: "d", status: "offline", enabled: true }, { id: "e", status: "online", enabled: true }]);
  gates.splice(0).forEach((open) => open());
  await Promise.all((await Promise.all(busy)).map((response) => response.text()));
  await flush();
  expect(sent).not.toContain("/api/environments/d/volumes/backups/overview");
  const skipped = await tagged;
  expect(skipped.status).toBe(502);
  expect(skipped.headers.get("X-Docklet-Skipped")).toBe("host");
  expect(await skipped.json()).toMatchObject({ code: "agent.unavailable" });
  expect(sent).not.toContain("/api/environments/d/volumes/backups/overview");
  // An untagged read (a user gesture) still reaches the host.
  await flush(); gates.splice(0).forEach((open) => open());
  expect((await gesture).status).toBe(200);
  expect(sent).toContain("/api/environments/d/containers/detail");
  // The tag never reaches the network.
  expect(physical.mock.calls.some(([, init]) => new Headers(init?.headers).has(READ_TAG_HEADER))).toBe(false);
});

it.each([
  ["pending", "background", true], ["update-required", "background", true], ["token-rejected", "background", true],
  ["online", "poll", false], ["standby", "poll", true], ["standby", "background", false],
])("R2-h host status %s skips a %s read: %s", (status, tag, skip) => {
  const policy = createHostReadPolicy();
  policy.applyStream([{ id: "e", status, enabled: true }]);
  expect(policy.skip("e", tag as "poll" | "background")).toBe(skip);
});

it("R2-g a disabled host is skipped and an unknown host is read", () => {
  const policy = createHostReadPolicy();
  policy.applyStream([{ id: "d", status: "online", enabled: false }]);
  expect(policy.skip("d", "background")).toBe(true);
  expect(policy.skip("x", "background")).toBe(false);
});

it("R2-g a catalogue list answer started before the last stream envelope does not undo the newer state", () => {
  const policy = createHostReadPolicy();
  const stale = policy.beginCatalogue();
  policy.applyStream([{ id: "d", status: "offline", enabled: true }]);
  policy.applyCatalogue(stale, [{ id: "d", status: "online", enabled: true }]);
  expect(policy.skip("d", "background")).toBe(true);
  const fresh = policy.beginCatalogue();
  policy.applyCatalogue(fresh, [{ id: "d", status: "online", enabled: true }]);
  expect(policy.skip("d", "background")).toBe(false);
});
