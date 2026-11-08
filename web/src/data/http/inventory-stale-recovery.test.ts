// Source: docklet-next/issues/271
// Revision: cd7a7d1b6b83a297a7657f4aecb524729661d2e4
// Retrieved: 2026-11-08
// License: Apache-2.0
// Deviations: Original stale-success recovery coverage across every inventory aggregate.

import { expect, it } from "vitest";
import { inventoryRecoveryInterval, inventoryStructuralSharing } from "../inventory-recovery";
import { createResourceReader } from "./resources";
import { createUserInventory } from "./user-inventory";
import { securityFixture } from "./vulnerabilities.test-support";

async function fixture(status = "online") {
  const f = securityFixture(); f.inventory.state.hostIds = ["first"]; f.inventory.state.status = status;
  const hosts = await f.adapter.listHosts();
  const resources = createResourceReader("/api", f.fetcher as typeof fetch, async () => hosts, async () => {}, () => {});
  const user = createUserInventory("/api", f.fetcher as typeof fetch, async () => hosts, () => undefined, () => {}, async () => new Map());
  return { f, hosts, reads: {
    stacks: () => f.adapter.listStacks(), containers: () => f.adapter.listContainers(),
    images: () => f.adapter.listImages(), volumes: () => f.adapter.listVolumes(),
    networks: () => f.adapter.listNetworkSummaries(), topology: () => f.adapter.getTopology(),
    security: () => f.adapter.getSecurity(), userStacks: () => user.listStacks(),
    resourceVolumes: () => resources.listVolumes(),
  } };
}
type ReadName = keyof Awaited<ReturnType<typeof fixture>>["reads"];
const names: ReadName[] = ["stacks", "containers", "images", "volumes", "networks", "topology", "security", "userStacks", "resourceVolumes"];
function rowCount(result: Awaited<ReturnType<Awaited<ReturnType<typeof fixture>>["reads"][ReadName]>>) {
  return Array.isArray(result) ? result.length : "networks" in result ? result.networks.length : result.images.length;
}

it.each(names)("%s retains snapshot rows, recovers transient stale success, and stops on fresh completion", async (name) => {
  const { f, hosts, reads } = await fixture();
  f.inventory.state.reason = "agent.unavailable";
  const stale = await reads[name]();
  expect(rowCount(stale) > 0).toBe(true);
  expect(inventoryRecoveryInterval(stale, hosts)).toBe(15_000);
  // Another query's success cannot clear this query's recovery ownership.
  f.inventory.state.reason = "";
  await reads.images();
  expect(inventoryRecoveryInterval(stale, hosts)).toBe(15_000);
  const fresh = await reads[name]();
  expect(rowCount(fresh)).toBe(rowCount(stale));
  expect(inventoryRecoveryInterval(inventoryStructuralSharing(stale, fresh), hosts)).toBe(false);
});

it.each(names)("C2-3 %s has no recovery timer for permanent stale reasons", async (name) => {
  const { f, hosts, reads } = await fixture();
  for (const reason of ["agent.invalid-token", "agent.update-required"]) {
    f.inventory.state.reason = reason;
    expect(inventoryRecoveryInterval(await reads[name](), hosts)).toBe(false);
  }
});

it.each(names)("C2-3 %s never records recovery for an offline catalogue host", async (name) => {
  const { f, hosts, reads } = await fixture("offline");
  expect(hosts[0]!.status).toBe("offline");
  f.inventory.state.reason = "agent.unavailable";
  expect(inventoryRecoveryInterval(await reads[name](), [{ ...hosts[0]!, status: "online" }])).toBe(false);
});

it.each(names)("%s recovers even when a transient stale snapshot contains no rows", async (name) => {
  const { f, hosts, reads } = await fixture();
  f.inventory.state.reason = "agent.unavailable";
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input, init) => {
    const response = await original(input, init);
    if (!response.headers.has("X-Docklet-Host-Reason")) return response;
    const body = await response.json();
    if (Array.isArray(body.data)) { body.data = []; body.pagination.totalItems = 0; }
    return Response.json(body, { headers: response.headers });
  });
  const result = await reads[name]();
  expect(rowCount(result)).toBe(0);
  expect(inventoryRecoveryInterval(result, hosts)).toBe(15_000);
});
