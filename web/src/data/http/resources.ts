// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original resource reader over the shared inventory transport. M20 #360: web hardening step (#359 #24 #175 #177 #383 #421 #271). D4 #271: fair physical host reads and result-local recovery. C2 #271: recover transient stale-success host batches without discarding snapshot rows.

import { inventoryRecovery } from "../inventory-recovery";
import { inventoryReadBudget } from "./host-read-deadline";

import type { DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import type { Host, InventoryObservation, Topology } from "@/domain/model";
import { splitInventoryId } from "./inventory-mapping";
import { createInventoryTransport, inventoryFailure } from "./inventory-transport";
import { mapImage, mapTopology, mapVolumes } from "./resource-mapping";

function combine(observations: InventoryObservation[]): InventoryObservation {
  return { observedAt: observations.flatMap((o) => o.observedAt ? [o.observedAt] : []).sort()[0], stale: observations.some((o) => o.stale), reason: observations.find((o) => o.reason)?.reason };
}

export function createHostImageReader(baseUrl: string, fetcher: typeof fetch, onState: (hostId: string, state: InventoryObservation) => void) {
  const read = createObservedHostImageReader(baseUrl, fetcher, onState);
  return async (hostId: string, signal?: AbortSignal) => (await read(hostId, signal)).images;
}

/** Security can own one host's complete inventory lifetime; the observation belongs to this read, not the shared host snapshot. */
export function createObservedHostImageReader(baseUrl: string, fetcher: typeof fetch, onState: (hostId: string, state: InventoryObservation) => void) {
  const { list } = createInventoryTransport(baseUrl, fetcher, onState);
  return async (hostId: string, signal?: AbortSignal) => {
    const [images, projects, containers] = await Promise.all([
      list("list-images", hostId, "images", undefined, signal),
      list("list-projects", hostId, "projects", undefined, signal),
      list("list-containers", hostId, "containers", undefined, signal),
    ]);
    signal?.throwIfAborted();
    const inventory = combine([projects.observation, containers.observation, images.observation]);
    onState(hostId, inventory);
    return { inventory, images: images.rows.filter((i) => i.labels?.["io.docklet.system"] !== "agent").map((i) => mapImage(i, hostId, containers.rows, projects.rows, inventory)) };
  };
}

export function createResourceReader(baseUrl: string, fetcher: typeof fetch, hosts: () => Promise<Host[]>, requireCapability: (operation: string) => Promise<void>, onState: (hostId: string, state: InventoryObservation) => void): Pick<DockletAdapter, "listImages" | "listVolumes" | "getVolume" | "getTopology" | "listNetworkSummaries"> {
  const { read, list } = createInventoryTransport(baseUrl, fetcher, onState);
  const hostImages = createObservedHostImageReader(baseUrl, fetcher, onState);
  async function context(hostId: string, signal?: AbortSignal) {
    const [projects, containers] = await Promise.all([list("list-projects", hostId, "projects", undefined, signal), list("list-containers", hostId, "containers", undefined, signal)]);
    return { projects: projects.rows, containers: containers.rows, observations: [projects.observation, containers.observation] };
  }
  function observe(hostId: string, observations: InventoryObservation[]): InventoryObservation {
    const inventory = combine(observations);
    onState(hostId, inventory);
    return inventory;
  }
  async function aggregate<T>(operation: string, load: (host: Host, signal: AbortSignal) => Promise<{ rows: T[]; inventory: InventoryObservation }>, onPartial?: (rows: T[]) => void, parent?: AbortSignal): Promise<T[]> {
    const budget = inventoryReadBudget(parent);
    const scope = await budget(async () => { await requireCapability(operation); return hosts(); });
    const recovery = inventoryRecovery();
    const completed: T[] = [];
    const batches = await Promise.all(scope.map(async (host) => {
      try { const { rows, inventory } = await budget((signal) => load(host, signal)); recovery.observed(host, inventory); completed.push(...rows); onPartial?.([...completed]); return rows; }
      catch (error) {
        parent?.throwIfAborted(); recovery.failed(host, error);
        onState(host.id, inventoryFailure(host.id, error));
        return [];
      }
    }));
    return recovery.result(batches.flat());
  }
  const listVolumes: DockletAdapter["listVolumes"] = (onPartial, signal) => aggregate("listVolumes", async (h, owned) => {
    const [volumes, ctx] = await Promise.all([list("list-volumes", h.id, "volumes", undefined, owned), context(h.id, owned)]);
    const inventory = observe(h.id, [...ctx.observations, volumes.observation]);
    return { rows: mapVolumes(volumes.rows, h.id, ctx.containers, ctx.projects, inventory), inventory };
  }, onPartial, signal);
  return {
    listVolumes,
    async getVolume(id) {
      await requireCapability("getVolume");
      splitInventoryId(id);
      const volume = (await listVolumes()).find((v) => v.id === id);
      if (!volume) throw new AdapterOutcomeError({ kind: "error", code: "get-volume.not-found" });
      return { ...volume, backups: [] };
    },
    listImages: (onPartial, signal) => aggregate("listImages", async (h, owned) => { const { images, inventory } = await hostImages(h.id, owned); return { rows: images, inventory }; }, onPartial, signal),
    listNetworkSummaries: (signal) => aggregate("getTopology", async (h, owned) => {
      const networks = await list("list-networks", h.id, "networks", undefined, owned);
      return { rows: networks.rows.filter((n) => n.labels?.["io.docklet.system"] !== "agent").map((n) => ({ id: n.id, hostId: h.id })), inventory: networks.observation };
    }, undefined, signal),
    async getTopology(onPartial, retained) {
      await requireCapability("getTopology");
      const scope = await hosts();
      const recovery = inventoryRecovery();
      // Query owns the retained snapshot; each host replaces only its own batch.
      const batches = new Map<string, Topology>(scope.map((h) => {
        const networks = retained?.networks.filter((n) => n.hostId === h.id) ?? [];
        const ids = new Set(networks.map((n) => n.id));
        const state = retained?.inventory?.[h.id];
        return [h.id, { networks, attachments: retained?.attachments.filter((a) => ids.has(a.networkId)) ?? [], dependencies: retained?.dependencies.filter((d) => d.from.startsWith(`${h.id}~`)) ?? [], ...(state ? { inventory: { [h.id]: state } } : {}) }];
      }));
      const merge = (): Topology => {
        const graphs = [...batches.values()];
        const inventory = Object.assign({}, ...graphs.map((g) => g.inventory));
        return { networks: graphs.flatMap((g) => g.networks), attachments: graphs.flatMap((g) => g.attachments), dependencies: graphs.flatMap((g) => g.dependencies), ...(Object.keys(inventory).length ? { inventory } : {}) };
      };
      await Promise.all(scope.map(async (h) => {
        try {
          const [listed, contextual] = await Promise.allSettled([list("list-networks", h.id, "networks"), context(h.id)]);
          if (listed.status === "rejected") throw listed.reason;
          const networks = listed.value;
          const ctx = contextual.status === "fulfilled" ? contextual.value : { containers: [], projects: [], observations: [inventoryFailure(h.id, contextual.reason)] };
          const details = await Promise.all(networks.rows.map(async (summary) => {
            try {
              const detail = await read("get-network", h.id, `networks/${encodeURIComponent(summary.id)}`);
              if (detail.body.data.id !== summary.id) throw new Error("inventory.invalid-network");
              // Invalid detail projections are local to this listed identity, too.
              mapTopology([detail.body.data], h.id, [], ctx.projects, detail.observation, [summary]);
              return { row: detail.body.data, observation: detail.observation, unavailable: false };
            } catch (error) {
              return { row: summary, observation: inventoryFailure(h.id, error), unavailable: true };
            }
          }));
          const base = combine([...ctx.observations, networks.observation, ...details.filter((d) => !d.unavailable).map((d) => d.observation)]);
          const unavailable = new Map(details.filter((d) => d.unavailable || contextual.status === "rejected").map((d) => [d.row.id, { ...base, ...d.observation, stale: true, reason: d.observation.reason ?? base.reason ?? "inventory.read-error" as const }]));
          const inventory = observe(h.id, [...ctx.observations, networks.observation, ...details.map((d) => d.observation)]);
          const graph = mapTopology(details.map((d) => d.row), h.id, ctx.containers, ctx.projects, base, networks.rows, unavailable);
          // Failed details retain their summaries without polling; successful stale reads still own recovery.
          recovery.observed(h, combine([...(contextual.status === "fulfilled" ? ctx.observations : []), networks.observation, ...details.filter((d) => !d.unavailable).map((d) => d.observation)]));
          batches.set(h.id, { ...graph, ...(inventory.stale ? { inventory: { [h.id]: inventory } } : {}) });
        } catch (error) {
          recovery.failed(h, error);
          const state = inventoryFailure(h.id, error);
          const previous = batches.get(h.id)!;
          onState(h.id, state);
          batches.set(h.id, { ...previous, inventory: { [h.id]: state }, networks: previous.networks.map((n) => ({ ...n, detailUnavailable: true, inventory: { ...n.inventory, ...state } })) });
        }
        onPartial?.(merge());
      }));
      return recovery.result(merge());
    },
  };
}
