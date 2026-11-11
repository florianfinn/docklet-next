// Source: docklet-next/issues/428
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original adapter-boundary scope, reason, retention and lifetime regression coverage. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags.

import { expect, it, vi } from "vitest";
import { inventoryRecoveryInterval } from "../inventory-recovery";
import { resourceFixture } from "./resources.test-support";
import { createInventoryTransport } from "./inventory-transport";

it.each([false, true])("L4-g resource combine keeps visible refusal and recovery, reverse=%s", async (reverse) => {
  const f = resourceFixture(); f.state.hostIds = ["first"];
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input) => {
    const answer = await original(input);
    const headers = new Headers(answer.headers);
    if (String(input).includes("/projects?")) headers.set("X-Docklet-Host-Reason", reverse ? "agent.unavailable" : "agent.update-required");
    if (String(input).includes("/containers?")) headers.set("X-Docklet-Host-Reason", reverse ? "agent.update-required" : "agent.unavailable");
    return new Response(await answer.text(), { headers, status: answer.status });
  });
  const rows = await f.adapter.listImages();
  expect(rows[0]?.inventory).toMatchObject({ reason: "agent.update-required", recovery: "agent.unavailable" });
  expect(inventoryRecoveryInterval(rows, await f.adapter.listHosts())).toBe(15_000);
});

it.each([false, true])("L4-g inventory merge keeps visible refusal and recovery, reverse=%s", async (reverse) => {
  const f = resourceFixture(); f.state.hostIds = ["first"];
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input) => {
    const answer = await original(input), headers = new Headers(answer.headers);
    if (String(input).includes("/projects?")) headers.set("X-Docklet-Host-Reason", reverse ? "agent.unavailable" : "agent.update-required");
    if (String(input).includes("/containers?")) headers.set("X-Docklet-Host-Reason", reverse ? "agent.update-required" : "agent.unavailable");
    return new Response(await answer.text(), { headers, status: answer.status });
  });
  const rows = await f.adapter.listContainers();
  expect(rows[0]?.inventory).toMatchObject({ reason: "agent.update-required", recovery: "agent.unavailable" });
  expect(inventoryRecoveryInterval(rows, await f.adapter.listHosts())).toBe(15_000);
});

it.each([false, true])("L4-g transport page merge keeps both reasons, reverse=%s", async (reverse) => {
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const start = new URL(String(input), "https://hub.example.com").searchParams.get("start");
    return Response.json({ success: true, data: [{ id: start }], pagination: { totalItems: 2 } }, { headers: { "X-Docklet-Host-Reason": (start === "0") !== reverse ? "agent.update-required" : "agent.unavailable" } });
  });
  const { observation } = await createInventoryTransport("/api", fetcher, () => {}).list("list-containers", "first", "containers");
  expect(observation).toMatchObject({ reason: "agent.update-required", recovery: "agent.unavailable" });
});
