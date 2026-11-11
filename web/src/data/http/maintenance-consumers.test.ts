// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original consumer proofs for maintenance refusals and skipped host reads (#389, #391; S2 class escalation).

import { QueryObserver } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";

import { createAppQueryClient } from "../app-query-client";
import { AdapterOutcomeError } from "../capability";
import { MAINTENANCE_RETRY_DELAY_MS } from "../maintenance-retry";
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
