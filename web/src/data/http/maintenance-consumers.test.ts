// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original consumer proofs for maintenance refusals and skipped host reads (#389, #391; S2 class escalation).

import { QueryObserver } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";

import { createAppQueryClient } from "../app-query-client";
import { AdapterOutcomeError } from "../capability";
import { INVENTORY_RECOVERY_MS, inventoryRecoveryInterval } from "../inventory-recovery";
import type { DockletAdapter } from "../adapter";
import { isMaintenanceError, MAINTENANCE_RETRY_DELAY_MS } from "../maintenance-retry";
import type { Host } from "@/domain/model";
import { createCsrfClient } from "./csrf-client";
import { maintenanceResponse } from "./dispatch-gate";
import { createScheduleReader } from "./inventory-schedule";
import { backupFixture } from "./backups.test-support";
import { securityFixture } from "./vulnerabilities.test-support";

afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); });
const maintenance = () => new AdapterOutcomeError({ kind: "error", code: "backup.maintenance", status: 503 });
const deepFreeze = <T>(value: T): T => {
  if (value && typeof value === "object" && !Object.isFrozen(value)) { Object.freeze(value); for (const child of Object.values(value)) deepFreeze(child); }
  return value;
};

it("R1B-5 a refetch refused by maintenance keeps cached data and success state while it retries, then recovers", async () => {
  vi.useFakeTimers();
  const client = createAppQueryClient();
  let calls = 0;
  const queryFn = vi.fn(async () => { calls++; if (calls === 1) return ["cached"]; if (calls <= 3) throw maintenance(); return ["fresh"]; });
  const observer = new QueryObserver(client, { queryKey: ["hosts"], queryFn, staleTime: 0 });
  const states: Array<{ status: string; data: unknown; isError: boolean }> = [];
  const stop = observer.subscribe((result) => states.push({ status: result.status, data: result.data, isError: result.isError }));
  await vi.advanceTimersByTimeAsync(0);
  expect(observer.getCurrentResult().data).toEqual(["cached"]);
  states.length = 0;
  void observer.refetch();
  await vi.advanceTimersByTimeAsync(MAINTENANCE_RETRY_DELAY_MS * 2 + 10);
  expect(states.some((state) => state.isError || state.data === undefined)).toBe(false);
  expect(observer.getCurrentResult()).toMatchObject({ status: "success", data: ["fresh"] });
  stop();
});

it("R1B-5 an ordinary read failure is not retried", async () => {
  const client = createAppQueryClient();
  const queryFn = vi.fn(async () => { throw new AdapterOutcomeError({ kind: "error", code: "http.error", status: 500 }); });
  await expect(client.fetchQuery({ queryKey: ["x"], queryFn })).rejects.toBeInstanceOf(AdapterOutcomeError);
  expect(queryFn).toHaveBeenCalledTimes(1);
});

it("R1B-4 a security read skipped for an offline host keeps that host's previous rows as unavailable new objects", async () => {
  const f = securityFixture();
  const before = deepFreeze(await f.adapter.getSecurity());
  const previous = before.images.filter((image) => image.hostId === "second");
  expect(previous.length).toBeGreaterThan(0);
  f.state.hostStatuses.second = "offline";
  // The catalogue read after its stale time reports the host offline (the environment stream would do so at once).
  const now = Date.now();
  vi.spyOn(Date, "now").mockReturnValue(now + 11_000);
  const sent: string[] = [];
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => { sent.push(new URL(String(input), "https://hub.example.com").pathname); return original(input, init); });
  const after = await f.adapter.getSecurity();
  const retained = after.images.filter((image) => image.hostId === "second");
  expect(retained.map((image) => image.id)).toEqual(previous.map((image) => image.id));
  expect(retained.every((image) => image.unavailable)).toBe(true);
  expect(retained.some((image) => previous.includes(image))).toBe(false);
  expect(sent.some((path) => path.startsWith("/api/environments/second/") && path.includes("/vulnerabilities"))).toBe(false);
});

it("R1B-8 the volume backup adapter reads maintenance from code, nested data.code and a problem detail", async () => {
  for (const body of [{ code: "backup.maintenance" }, { success: false, data: { code: "backup.maintenance" } }, { status: 503, detail: "backup.maintenance" }]) {
    const f = backupFixture();
    f.intercept(async (_url, init) => init?.method === "POST" ? Response.json(body, { status: 503 }) : undefined);
    await expect(f.api.runBackup("first~data")).rejects.toMatchObject({ outcome: { code: "backup.maintenance" } });
  }
});

/** The security fixture's hub with a maintenance status endpoint and maintenance answers while it is active. */
function maintainedSecurity() {
  const f = securityFixture();
  const original = f.fetcher.getMockImplementation()!;
  const m = { active: false };
  f.fetcher.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path === "/api/backups/maintenance") return Response.json({ active: m.active, operation: "backup" });
    if (m.active && !path.startsWith("/api/auth/")) return Response.json({ status: 503, detail: "backup.maintenance" }, { status: 503 });
    return original(input, init);
  });
  return { f, m };
}

it("R1B-1 a security read held 45 s by maintenance ends with live rows, not an aborted unavailable result", async () => {
  vi.useFakeTimers();
  const { f, m } = maintainedSecurity();
  const live = await f.adapter.getSecurity();
  expect(live.images.some((i) => !i.unavailable)).toBe(true);
  m.active = true;
  await f.adapter.getHubBackupMaintenance();
  const read = f.adapter.getSecurity();
  await vi.advanceTimersByTimeAsync(45_000);
  m.active = false;
  await vi.advanceTimersByTimeAsync(2_000);
  const after = await read.then((value) => value, (error: unknown) => ({ rejected: String(error) }));
  expect("rejected" in after ? after : [after.unavailable, after.images.every((i) => !i.unavailable)]).toEqual([[], true]);
});

it("R1B-4 a stale online catalogue with an offline stream row: the security read is skipped, keeps frozen rows as new unavailable objects, reports agent.unavailable and schedules recovery", async () => {
  const f = securityFixture();
  const stop = f.adapter.subscribeChanges(() => {});
  const before = deepFreeze(await f.adapter.getSecurity());
  const previous = before.images.filter((image) => image.hostId === "second");
  const hosts = await f.adapter.listHosts();
  const sent: string[] = [];
  let answerCatalogue!: () => void;
  const catalogueHeld = new Promise<void>((resolve) => { answerCatalogue = resolve; });
  let catalogueAsked!: () => void;
  const asked = new Promise<void>((resolve) => { catalogueAsked = resolve; });
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    sent.push(path);
    // The catalogue answer was produced before the host went offline: it still says online.
    if (path === "/api/environments") { catalogueAsked(); await catalogueHeld; }
    return original(input, init);
  });
  const envelope = { channel: "environments", timestamp: "2026-11-11T12:00:00Z", environment: { environments: hosts.map((h) => ({ id: h.id, name: h.id, status: h.id === "second" ? "offline" : "online", enabled: true, apiUrl: "http://agent.example.com", isEdge: false })) } };
  // The stream row invalidates the catalogue; the next read's catalogue request is in flight when it is answered stale.
  f.source.onmessage?.({ data: JSON.stringify(envelope), lastEventId: "" } as MessageEvent);
  const read = f.adapter.getSecurity();
  await asked;
  f.source.onmessage?.({ data: JSON.stringify(envelope), lastEventId: "" } as MessageEvent);
  answerCatalogue();
  const after = await read;
  const retained = after.images.filter((image) => image.hostId === "second");
  expect(retained.length > 0 && retained.map((image) => image.id).join() === previous.map((image) => image.id).join()).toBe(true);
  expect(retained.every((image) => image.unavailable) && !retained.some((image) => previous.includes(image))).toBe(true);
  expect((after.unavailable ?? []).map((u) => [u.hostId, u.code])).toEqual([["second", "agent.unavailable"]]);
  expect(sent.some((path) => path.startsWith("/api/environments/second/") && path.includes("vulnerabilities"))).toBe(false);
  expect(inventoryRecoveryInterval(after, hosts)).toBe(INVENTORY_RECOVERY_MS);
  stop();
});

it("E1/B19 a schedule read whose update policy part meets hub maintenance rejects with maintenance, keeping the cached schedule", async () => {
  const base = "https://hub.example.com/api";
  // Backup overviews answer; the updater read gets the dispatch gate's maintenance answer.
  const client = createCsrfClient(base, async (input) => String(input).endsWith("/auth/mode")
    ? Response.json({ success: true, data: { mode: "local" } }, { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } })
    : maintenanceResponse());
  const host = { id: "first", name: "first", status: "online" } as Host;
  const read = createScheduleReader(base, client, async () => [host], async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }), () => 0,
    async () => ({ volumes: [] }) as unknown as Awaited<ReturnType<NonNullable<DockletAdapter["getBackupOverview"]>>>, async () => ({ timeZone: "UTC" }) as never);
  const outcome = await read(["first"]).then((rows) => rows, (error: unknown) => error);
  expect(isMaintenanceError(outcome)).toBe(true);
});
