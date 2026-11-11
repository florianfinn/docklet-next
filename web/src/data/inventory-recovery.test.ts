// Source: docklet-next/issues/271
// Revision: bbab488e30d2e91edbd840bca929737c3ac1ab5c
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original query-local recovery, equal-result sharing and catalogue exclusion regressions. C2 #271: stale-success catalogue and freshness exclusions. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags.

import { expect, it } from "vitest";
import type { Host } from "@/domain/model";
import { AdapterOutcomeError } from "./capability";
import { inventoryRecovery, inventoryRecoveryInterval, inventoryStructuralSharing } from "./inventory-recovery";
import { createResourceReader } from "./http/resources";
import { createUserInventory } from "./http/user-inventory";
import { resourceFixture } from "./http/resources.test-support";

const host = { id: "direct", status: "online" } as Host;
it.each(["http.error", "agent.unavailable", "inventory.read-error", "vulnerability.read-timeout"])("recovers %s only for this result and while the host remains online", (code) => {
  const recovery = inventoryRecovery(); recovery.failed(host, new AdapterOutcomeError({ kind: "error", code }));
  const rows = recovery.result([]);
  expect(inventoryRecoveryInterval(rows, [host])).toBe(15_000);
  expect(inventoryRecoveryInterval([], [host])).toBe(false);
  expect(inventoryRecoveryInterval(rows, [])).toBe(false);
  expect(inventoryRecoveryInterval(rows, [{ ...host, status: "offline" }])).toBe(false);
});
it.each(["offline", "pending", "disabled"] as const)("never retries a %s catalogue host", (status) => {
  const recovery = inventoryRecovery(); recovery.failed({ ...host, status } as Host, "agent.unavailable");
  expect(inventoryRecoveryInterval(recovery.result([]), [host])).toBe(false);
  recovery.observed({ ...host, status } as Host, { stale: true, reason: "agent.unavailable" });
  expect(inventoryRecoveryInterval(recovery.result([]), [host])).toBe(false);
});
it("C2-3 does not treat a fresh observation with a leftover reason as a failed batch", () => {
  const recovery = inventoryRecovery(); recovery.observed(host, { stale: false, reason: "agent.unavailable" });
  expect(inventoryRecoveryInterval(recovery.result([]), [host])).toBe(false);
});
it.each(["inventory.mapping-error", "permission.denied", "agent.invalid-token", "agent.update-required"])("does not retry permanent read failure %s", (code) => {
  const recovery = inventoryRecovery(); recovery.failed(host, new AdapterOutcomeError({ kind: "error", code }));
  expect(inventoryRecoveryInterval(recovery.result([]), [host])).toBe(false);
});
it("transfers and clears failure ownership when structural sharing reuses an equal empty array", () => {
  const previous: unknown[] = [], recovery = inventoryRecovery(); recovery.failed(host, "http.error");
  const shared = inventoryStructuralSharing(previous, recovery.result([]));
  expect(shared === previous).toBe(true); expect(inventoryRecoveryInterval(shared, [host])).toBe(15_000);
  const healthy = inventoryStructuralSharing(shared, inventoryRecovery().result([]));
  expect(healthy === shared).toBe(true); expect(inventoryRecoveryInterval(healthy, [host])).toBe(false);
});
it("propagates the resource volume query's cancellation to every physical context read", async () => {
  const f = resourceFixture(); f.state.hostIds = ["first"];
  const hosts = await f.adapter.listHosts(); const signals: AbortSignal[] = [];
  const reader = createResourceReader("/api", async (_input, init) => {
    signals.push(init!.signal!);
    return new Promise<Response>((_resolve, reject) => init!.signal!.addEventListener("abort", () => reject(init!.signal!.reason), { once: true }));
  }, async () => hosts, async () => {}, () => {});
  const parent = new AbortController(), pending = reader.listVolumes(undefined, parent.signal).catch((error: unknown) => error);
  for (let i = 0; i < 20; i++) await Promise.resolve();
  parent.abort(); await pending;
  expect(signals.length).toBe(3); expect(signals.every((signal) => signal.aborted)).toBe(true);
});

it("keeps an unrelated successful inventory from clearing another list's recovery", () => {
  const failed = inventoryRecovery(); failed.failed(host, "http.error");
  const containers = failed.result([]); const images = inventoryRecovery().result([]);
  expect(inventoryRecoveryInterval(images, [host])).toBe(false);
  expect(inventoryRecoveryInterval(containers, [host])).toBe(15_000);
});

// A failed network detail keeps the host's summary rows without scheduling a host-batch re-read.
it.each([["batch", "/networks", 15_000], ["detail", "/networks/net", false]] as const)("topology recovers a failed %s only when the host batch failed", async (_, failing, interval) => {
  const f = resourceFixture(); f.state.hostIds = ["first"];
  const hosts = await f.adapter.listHosts();
  const fetcher: typeof fetch = async (input) => new URL(String(input), "https://hub.example.com").pathname.endsWith(failing)
    ? Response.json({ success: false, code: "agent.unavailable" }, { status: 502 }) : f.fetcher(input as string);
  const reader = createResourceReader("/api", fetcher, async () => hosts, async () => {}, () => {});
  expect(inventoryRecoveryInterval(await reader.getTopology(), hosts)).toBe(interval);
});

it("an assigned-stack list recovers a host whose project batch failed", async () => {
  const f = resourceFixture(); f.state.hostIds = ["first"];
  const hosts = await f.adapter.listHosts();
  const fetcher: typeof fetch = async (input) => String(input).includes("/projects")
    ? Response.json({ success: false, code: "agent.unavailable" }, { status: 502 }) : f.fetcher(input as string);
  const reader = createUserInventory("/api", fetcher, async () => hosts, () => undefined, () => {}, async () => new Map());
  expect(inventoryRecoveryInterval(await reader.listStacks(), hosts)).toBe(15_000);
});

it("an assigned-stack list publishes its own stale and fresh observations", async () => {
  const f = resourceFixture(); f.state.hostIds = ["first"];
  const hosts = await f.adapter.listHosts();
  const observations: boolean[] = [];
  const reader = createUserInventory("/api", f.fetcher as typeof fetch, async () => hosts, () => undefined, (_host, state) => observations.push(state.stale), async () => new Map());
  f.state.reason = "agent.unavailable";
  await reader.listStacks();
  f.state.reason = "";
  await reader.listStacks();
  expect(observations).toEqual([true, false]);
});

