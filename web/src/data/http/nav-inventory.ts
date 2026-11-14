// Source: docklet-next/issues/462
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original same-cycle host volume counters and unfiltered running inventory.

import { CancelledError } from "@tanstack/react-query";
import type { DockletAdapter } from "../adapter";
import { AdapterOutcomeError } from "../capability";
import { inventoryRecovery } from "../inventory-recovery";
import { distinctBindCount, type NavInventory } from "../nav-inventory";
import type { Host, InventoryObservation } from "@/domain/model";
import { inventoryReadBudget } from "./host-read-deadline";
import { createInventoryTransport, inventoryFailure } from "./inventory-transport";
import { mapContainer } from "./inventory-mapping";
import { isMaintenanceError } from "../maintenance-retry";

/** Host registration changes invalidate retained totals without discarding counts on availability changes. */
export function createNavInventoryState() {
  let registrations = new Map<string, string>(), totals = new Map<string, number>();
  return {
    get totals() { return totals; },
    registration: (host: string) => registrations.get(host),
    reset() { registrations = new Map(); totals = new Map(); },
    observe(rows: { id: string; registrationRevision?: number; apiUrl?: string; isEdge?: boolean }[]) {
      const next = new Map(rows.map((h) => [h.id, JSON.stringify([h.id, h.registrationRevision, h.apiUrl, h.isEdge])]));
      for (const [id, identity] of registrations) if (next.get(id) !== identity) totals.delete(id);
      registrations = next;
    },
  };
}

export function createNavInventory(baseUrl: string, fetcher: typeof fetch, hosts: () => Promise<Host[]>, requireCapability: (operation: string) => Promise<void>, observe: (host: string, state: InventoryObservation) => void, generation: () => number, totals: Map<string, number>, registration: (host: string) => string | undefined = () => undefined): Pick<DockletAdapter, "listNavInventory"> {
  const transport = createInventoryTransport(baseUrl, fetcher, observe);
  return {
    async listNavInventory(onPartial, parent) {
      const version = generation();
      const check = () => { parent?.throwIfAborted(); if (version !== generation()) throw new CancelledError(); };
      const budget = inventoryReadBudget(parent), recovery = inventoryRecovery();
      const scope = await budget(async () => { await requireCapability("listVolumes"); await requireCapability("listContainers"); return hosts(); });
      check();
      const batches = new Map<string, NavInventory>();
      for (const id of totals.keys()) if (!scope.some((host) => host.id === id)) totals.delete(id);
      await Promise.all(scope.map(async (host) => {
        const owner = registration(host.id);
        const current = () => { check(); if (owner !== registration(host.id)) throw new CancelledError(); };
        const batch: NavInventory = { hostId: host.id, containers: [], inventoryUnavailable: true, volumeTotal: totals.get(host.id) };
        // Reuse the container read already in flight before admitting another host request.
        const inventoryRead = budget(async (signal) => {
          const result = await transport.list("list-containers", host.id, "containers", undefined, signal);
          return { containers: result.rows.map((row) => mapContainer(row, host.id, [], result.observation)), observation: result.observation };
        });
        const [inventory, count] = await Promise.allSettled([
          inventoryRead,
          inventoryRead.catch(() => undefined).then(() => { current(); return budget(async (signal) => {
            const result = await transport.read("get-volume-usage-counts", host.id, "volumes/_collection/counts", undefined, undefined, signal);
            const total = result.body.data?.total;
            if (!Number.isSafeInteger(total) || total < 0) throw new AdapterOutcomeError({ kind: "error", code: "inventory.invalid-volume-count" });
            return { total, observation: result.observation };
          }); }),
        ]);
        current();
        for (const leg of [inventory, count]) {
          if (leg.status === "rejected") {
            if (isMaintenanceError(leg.reason)) throw leg.reason;
            recovery.failed(host, leg.reason);
          } else recovery.observed(host, leg.value.observation);
        }
        if (inventory.status === "fulfilled") {
          batch.containers = inventory.value.containers;
          batch.inventoryUnavailable = inventory.value.observation.stale;
          batch.inventory = inventory.value.observation;
        } else batch.inventory = inventoryFailure(host.id, inventory.reason);
        // Neither a new bind set with an old Docker count nor the reverse is a valid host total.
        if (inventory.status === "fulfilled" && count.status === "fulfilled" && !inventory.value.observation.stale && !count.value.observation.stale) {
          batch.volumeTotal = count.value.total + distinctBindCount(inventory.value.containers);
          current(); totals.set(host.id, batch.volumeTotal);
        }
        current(); batches.set(host.id, batch);
        onPartial?.([...batches.values()]);
      }));
      check();
      return recovery.result([...batches.values()]);
    },
  };
}
