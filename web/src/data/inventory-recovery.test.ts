// Source: docklet-next/issues/271
// Revision: bbab488e30d2e91edbd840bca929737c3ac1ab5c
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original query-local recovery, equal-result sharing and catalogue exclusion regressions.

import { expect, it } from "vitest";
import type { Host } from "@/domain/model";
import { AdapterOutcomeError } from "./capability";
import { inventoryRecovery, inventoryRecoveryInterval, inventoryStructuralSharing } from "./inventory-recovery";
import { createResourceReader } from "./http/resources";
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
