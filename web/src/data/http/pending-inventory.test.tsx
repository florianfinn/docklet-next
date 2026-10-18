// Source: docklet-next/issues/85
// Revision: a97d94423e731203eaf5bdcfe96e7744ce592fa2
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original pending-registration inventory request tests.

import { act } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { resourceFixture } from "./resources.test-support";

afterEach(() => vi.useRealTimers());

it.each([false, true])("A-N5 reads pending registrations without snapshots while other hosts remain populated (Edge=%s)", async (isEdge) => {
 vi.useFakeTimers();
 const f = resourceFixture();
 const original = f.fetcher.getMockImplementation()!;
 f.fetcher.mockImplementation(async (input) => {
  const url = new URL(String(input), "https://hub.example.com");
  if (url.pathname.startsWith("/api/environments/second/")) return new Response(JSON.stringify({ success: false, code: "agent.unavailable" }), { status: 502 });
  const response = await original(input);
  if (url.pathname === "/api/capabilities") {
   const body = await response.json();
   body.data.push({ operationId: "getStack", available: true, resourceRef: { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" } });
   return new Response(JSON.stringify(body), { headers: response.headers });
  }
  if (url.pathname !== "/api/environments") return response;
  const body = await response.json();
  body.data = body.data.map((row: { id: string }) => ({ ...row, isEdge, ...(row.id === "second" ? { status: "pending" } : {}) }));
  return new Response(JSON.stringify(body), { headers: response.headers });
 });
 const { adapter } = f;
 const reads = () => f.fetcher.mock.calls.filter(([url]) => String(url).includes("/environments/second/"));
 const poll = () => Promise.all([adapter.listStacks(), adapter.listContainers(), adapter.listImages(), adapter.listVolumes(), adapter.getTopology()]);
 const stop = adapter.subscribeMetrics(() => {});
 try {
  for (let interval = 0; interval < 3; interval++) {
   const [stacks, containers, images, volumes, topology] = await poll();
   for (const rows of [stacks, containers, images, volumes, topology.networks]) {
    expect(rows.length).toBeGreaterThan(0);
    expect(rows.every((row) => row.hostId === "first")).toBe(true);
   }
   await expect(adapter.getStack("second~project")).rejects.toMatchObject({ outcome: { code: "agent.unavailable" } });
   await expect(adapter.getVolume("second~data")).rejects.toBeDefined();
   await act(() => vi.advanceTimersByTimeAsync(10_000));
  }
  expect(reads().length).toBeGreaterThan(0);
  const host = (await adapter.listHosts()).find((row) => row.id === "second");
  expect(host?.inventory).toEqual({ stale: true, reason: "agent.unavailable" });
  expect(f.webSocket.mock.calls.some(([url]) => url.includes("second"))).toBe(false);
  // The next environment snapshot admits the host as soon as pairing completes.
  f.fetcher.mockImplementation(original);
  await act(() => vi.advanceTimersByTimeAsync(10_000));
  const [stacks, containers, images, volumes, topology] = await poll();
  for (const rows of [stacks, containers, images, volumes, topology.networks]) expect(rows.some((row) => row.hostId === "second")).toBe(true);
  expect(reads().length).toBeGreaterThan(0);
 } finally { stop(); }
});
