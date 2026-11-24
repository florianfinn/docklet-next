// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original resource inventory contract tests.

import { screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { MetricsSnapshot } from "@/domain/model";
import { InventoryNotice } from "@/features/stacks/InventoryNotice";
import { renderWithApp } from "@/test/render";
import { at, resourceFixture } from "./resources.test-support";

afterEach(() => vi.useRealTimers());

const operations = ["listStacks", "listContainers", "listImages", "listVolumes", "getTopology"] as const;
const KINDS = { listStacks: "stacks", listContainers: "containers", listImages: "images", listVolumes: "volumes", getTopology: "networks" } as const;
it.each(operations.flatMap((operation) => ["http", "mapping"].map((failure) => ({ operation, failure }))))("isolates $failure failures in $operation and renders the host reason", async ({ operation, failure }) => {
  const f = resourceFixture(); f.state.failure = failure;
  const data = await f.adapter[operation]();
  const rows = Array.isArray(data) ? data : data.networks;
  expect(rows.length).toBeGreaterThan(0);
  expect(rows.every((row) => row.hostId === "first")).toBe(true);
  const hosts = await f.adapter.listHosts();
  const kind = KINDS[operation];
  // #600: a row that cannot be mapped is left out and counted; only a failed read marks the whole host.
  // The volume reader also counts the consumer container and the bind metadata row it could not read.
  const count = operation === "listVolumes" ? 3 : 1;
  if (failure === "http") expect(hosts.find((h) => h.id === "second")?.inventory).toEqual({ stale: true, reason: "inventory.read-error" });
  // The topology also reads each listed network; that inspect of the malformed row fails and keeps its own stale mark.
  else expect(hosts.find((h) => h.id === "second")?.inventory).toMatchObject({ ...(operation === "getTopology" ? {} : { stale: false }), unreadable: { [kind]: { count } } });
  renderWithApp(<InventoryNotice hosts={hosts} items={rows} kinds={[kind]} />, f.adapter);
  expect(screen.getByRole("status").textContent).toContain("second");
  expect(screen.getByRole("status").textContent).toContain(failure === "http" ? "Inventar konnte nicht gelesen werden" : count === 1 ? "second: 1 Eintrag konnte nicht gelesen werden" : `second: ${count} Einträge konnten nicht gelesen werden`);
  expect(screen.getByRole("status").textContent).not.toContain("first:");
});

it.each(["http", "mapping"])("isolates %s failures in the metrics subscription without a global error", async (failure) => {
  vi.useFakeTimers(); const f = resourceFixture(); let snapshot: MetricsSnapshot | undefined;
  const errors = vi.fn(); window.addEventListener("docklet:adapter-error", errors);
  const stop = f.adapter.subscribeMetrics((s) => { snapshot = s; });
  try {
    await vi.advanceTimersByTimeAsync(0);
    f.sample("first", 25); f.sample("second", 50);
    expect(snapshot?.containers["second~web"]).toBeDefined();
    f.state.failure = failure;
    await vi.advanceTimersByTimeAsync(5000);
    expect(snapshot?.hosts.first?.cpu).toBe(25);
    expect(snapshot?.containers["first~web"]).toEqual({ cpu: 2.5, memMb: 100 });
    expect(Object.keys(snapshot?.containers ?? {}).some((id) => id.startsWith("second~"))).toBe(false);
    // #600: a malformed row drops only its own sample; only a failed read marks the host.
    const reason = failure === "http" ? "inventory.read-error" : undefined;
    expect((await f.adapter.listHosts()).find((h) => h.id === "second")?.inventory?.reason).toBe(reason);
    if (reason) expect(snapshot?.hosts.second?.inventory).toMatchObject({ stale: true, reason });
    expect(errors).not.toHaveBeenCalled();
  } finally { stop(); window.removeEventListener("docklet:adapter-error", errors); }
});

it("maps unavailable local Docker to the unavailable reason and skips offline metrics reads", async () => {
  vi.useFakeTimers(); const f = resourceFixture(); const stopChanges = f.adapter.subscribeChanges(() => {});
  f.state.hostIds = ["first", "0"]; f.state.failureHost = "0"; f.state.failure = "http";
  expect((await f.adapter.listStacks()).map((s) => s.hostId)).toEqual(["first"]);
  expect((await f.adapter.listHosts()).find((h) => h.id === "0")?.inventory?.reason).toBe("agent.unavailable");
  f.state.status = "offline"; f.source.onmessage?.({ data: JSON.stringify({ timestamp: at, channel: "environments" }), lastEventId: "" } as MessageEvent); f.fetcher.mockClear();
  const stop = f.adapter.subscribeMetrics(() => {});
  await vi.advanceTimersByTimeAsync(5000); stop();
  expect(f.fetcher.mock.calls.some(([url]) => String(url).includes("/containers"))).toBe(false);
  stopChanges();
});

it.each(["/api/environments", "/api/capabilities"])("keeps hub-level %s failures visible", async (route) => {
  const f = resourceFixture(); const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input) => new URL(String(input), "https://hub.example.com").pathname === route ? new Response("{}", { status: 500 }) : original(input));
  await expect(f.adapter.listStacks()).rejects.toBeDefined();
});
