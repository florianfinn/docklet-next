// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original resource reader over the shared inventory transport.

import type { DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import type { Host, InventoryObservation } from "@/domain/model";
import { splitInventoryId } from "./inventory-mapping";
import { createInventoryTransport, inventoryFailure } from "./inventory-transport";
import { mapImage, mapTopology, mapVolumes } from "./resource-mapping";

export function createResourceReader(baseUrl: string, fetcher: typeof fetch, hosts: () => Promise<Host[]>, requireCapability: (operation: string) => Promise<void>, onState: (hostId: string, state: InventoryObservation) => void): Pick<DockletAdapter, "listImages" | "listVolumes" | "getVolume" | "getTopology"> {
  const { read, list } = createInventoryTransport(baseUrl, fetcher, onState);
  async function context(hostId: string) {
    const [projects, containers] = await Promise.all([list("list-projects", hostId, "projects"), list("list-containers", hostId, "containers")]);
    return { projects: projects.rows, containers: containers.rows, observations: [projects.observation, containers.observation] };
  }
  function observe(hostId: string, observations: InventoryObservation[]): InventoryObservation {
    const inventory = { observedAt: observations.flatMap((o) => o.observedAt ? [o.observedAt] : []).sort()[0], stale: observations.some((o) => o.stale), reason: observations.find((o) => o.reason)?.reason };
    onState(hostId, inventory);
    return inventory;
  }
  async function aggregate<T>(operation: string, load: (host: Host) => Promise<T[]>, onPartial?: (rows: T[]) => void): Promise<T[]> {
    await requireCapability(operation);
    const completed: T[] = [];
    const batches = await Promise.all((await hosts()).map(async (host) => {
      try { const rows = await load(host); completed.push(...rows); onPartial?.([...completed]); return rows; }
      catch (error) {
        onState(host.id, inventoryFailure(host.id, error));
        return [];
      }
    }));
    return batches.flat();
  }
  const listVolumes: DockletAdapter["listVolumes"] = (onPartial) => aggregate("listVolumes", async (h) => {
    const [volumes, ctx] = await Promise.all([list("list-volumes", h.id, "volumes"), context(h.id)]);
    return mapVolumes(volumes.rows, h.id, ctx.containers, ctx.projects, observe(h.id, [...ctx.observations, volumes.observation]));
  }, onPartial);
  return {
    listVolumes,
    async getVolume(id) {
      await requireCapability("getVolume");
      splitInventoryId(id);
      const volume = (await listVolumes()).find((v) => v.id === id);
      if (!volume) throw new AdapterOutcomeError({ kind: "error", code: "get-volume.not-found" });
      return { ...volume, backups: [] };
    },
    listImages: (onPartial) => aggregate("listImages", async (h) => {
      const [images, ctx] = await Promise.all([list("list-images", h.id, "images"), context(h.id)]);
      const inventory = observe(h.id, [...ctx.observations, images.observation]);
      return images.rows.filter((i) => i.labels?.["io.docklet.system"] !== "agent").map((i) => mapImage(i, h.id, ctx.containers, ctx.projects, inventory));
    }, onPartial),
    async getTopology(onPartial) {
      const merge = (graphs: Awaited<ReturnType<typeof mapTopology>>[]) => ({ networks: graphs.flatMap((g) => g.networks), attachments: graphs.flatMap((g) => g.attachments), dependencies: graphs.flatMap((g) => g.dependencies) });
      const graphs = await aggregate("getTopology", async (h) => {
        const [networks, ctx] = await Promise.all([list("list-networks", h.id, "networks"), context(h.id)]);
        const details = await Promise.all(networks.rows.map((n) => read("get-network", h.id, `networks/${encodeURIComponent(n.id)}`)));
        const inventory = observe(h.id, [...ctx.observations, networks.observation, ...details.map((d) => d.observation)]);
        return [mapTopology(details.map((d) => d.body.data), h.id, ctx.containers, ctx.projects, inventory, networks.rows)];
      }, (graphs) => onPartial?.(merge(graphs)));
      return merge(graphs);
    },
  };
}
