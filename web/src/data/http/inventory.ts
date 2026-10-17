// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original multi-host inventory reader using existing agent forwarding.

import type { DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import type { Host, InventoryObservation } from "@/domain/model";
import { withoutSystem } from "@/domain/system";
import { createInventoryTransport, inventoryFailure } from "./inventory-transport";
import { inventoryId, mapContainer, mapStack, splitInventoryId, type ContainerRow, type ProjectRow } from "./inventory-mapping";

export function createInventoryReader(baseUrl: string, fetcher: typeof fetch, hosts: () => Promise<Host[]>, requireCapability: (operation: string) => Promise<void>, onState: (hostId: string, state: InventoryObservation) => void): Pick<DockletAdapter, "listStacks" | "listContainers" | "getStack"> {
  const { read, list } = createInventoryTransport(baseUrl, fetcher, onState);
  function fail(code: string): never { throw new AdapterOutcomeError({ kind: "error", code }); }
  async function hostInventory(host: Host) {
    try {
      const [projects, containers] = await Promise.all([list("list-projects", host.id, "projects"), list("list-containers", host.id, "containers")]);
      const observation: InventoryObservation = { ...projects.observation, ...(containers.observation.stale ? containers.observation : {}), stale: projects.observation.stale || containers.observation.stale };
      onState(host.id, observation);
      const projectRows = projects.rows as ProjectRow[];
      const containerRows = containers.rows as ContainerRow[];
      const allContainers = containerRows.map((row) => mapContainer(row, host.id, projectRows, observation));
      const stacks = withoutSystem(projectRows.map((row) => mapStack(row, host.id, allContainers, observation)));
      return { stacks, containers: withoutSystem(allContainers) };
    } catch (error) {
      onState(host.id, inventoryFailure(host.id, error));
      return { stacks: [], containers: [] };
    }
  }
  async function aggregate(operation: string, onPartial?: (batches: Awaited<ReturnType<typeof hostInventory>>[]) => void) {
    await requireCapability(operation);
    const batches: Awaited<ReturnType<typeof hostInventory>>[] = [];
    return Promise.all((await hosts()).map(async (host) => {
      const batch = await hostInventory(host);
      batches.push(batch);
      onPartial?.(batches);
      return batch;
    }));
  }
  return {
    async listStacks(onPartial) { return (await aggregate("listStacks", (batches) => onPartial?.(batches.flatMap((b) => b.stacks)))).flatMap((batch) => batch.stacks); },
    async listContainers(onPartial) { return (await aggregate("listContainers", (batches) => onPartial?.(batches.flatMap((b) => b.containers)))).flatMap((batch) => batch.containers); },
    async getStack(id) {
      await requireCapability("getStack");
      const [hostId, projectId] = splitInventoryId(id);
      const host = (await hosts()).find((item) => item.id === hostId);
      if (!host) return fail("get-project.host-not-found");
      const [detail, batch] = await Promise.all([read("get-project", hostId, `projects/${encodeURIComponent(projectId)}`), hostInventory(host)]);
      if (!detail.body.data || detail.body.data.id !== projectId) return fail("get-project.invalid-detail");
      if (!batch.stacks.some((item) => item.id === inventoryId(hostId, projectId))) return fail("get-project.not-found");
      return { ...mapStack(detail.body.data, hostId, batch.containers, detail.observation), compose: detail.body.data.composeContent ?? "", env: detail.body.data.envContent ?? "", deployments: [], restorePoints: [] };
    },
  };
}
