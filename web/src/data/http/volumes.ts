// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 1e03c054f5d2c355759cb79cbedfaf97706d4be1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: M23 #428 #389: the identity owner supplies retained volume rows across per-call readers. Original live volume mapping and regression coverage for issue #181. M20 #360: web hardening step (#359 #24 #175 #177 #383 #421 #271). D4 #271: fair physical host reads and result-local recovery. C2 #271: recover transient stale-success host batches without discarding snapshot rows. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags.

import { inventoryRecovery } from "../inventory-recovery";
import { inventoryReadBudget } from "./host-read-deadline";

import { CancelledError } from "@tanstack/react-query";

import type { components, operations } from "@/api/generated/schema";
import { AdapterError } from "@/data/api";
import type { DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import type { Host, InventoryObservation } from "@/domain/model";
import { bindSize, type BindInfo, type Volume } from "@/domain/volumes";
import { inventoryId, splitInventoryId } from "./inventory-mapping";
import { createInventoryTransport, inventoryFailure } from "./inventory-transport";
import { mapVolumes } from "./resource-mapping";
import { isMaintenanceError } from "../maintenance-retry";

type Read = "get-volume" | "get-volume-sizes" | "list-volume-binds";
type Body<I extends Read> = operations[I]["responses"][200]["content"]["application/json"];
type Schema = components["schemas"];

export function createVolumeReads(baseUrl: string, fetcher: typeof fetch, hosts: () => Promise<Host[]>, requireCapability: (operation: string) => Promise<void>, onState: (id: string, state: InventoryObservation) => void, generation: () => number, decorate: (host: string, rows: Volume[], signal: AbortSignal) => Promise<Volume[]> = async (_host, rows) => rows, retained = new Map<string, Volume[]>()): Pick<DockletAdapter, "listVolumes" | "getVolume"> {
  const transport = createInventoryTransport(baseUrl, fetcher, onState);
  let owner = generation();
  function begin() {
    const version = generation();
    if (owner !== version) { retained.clear(); owner = version; }
    return () => { if (generation() !== version) throw new CancelledError(); };
  }
  async function read<I extends Read>(operation: I, hostId: string, suffix: string, observed?: (state: InventoryObservation) => void, signal?: AbortSignal): Promise<Body<I>["data"]> {
    const response = await fetcher(`${baseUrl}/environments/${encodeURIComponent(hostId)}/volumes/${suffix}`, { credentials: "same-origin", headers: { Accept: "application/json" }, signal });
    let body: Body<I> & { code?: string };
    try { body = await response.json(); } catch { throw new AdapterOutcomeError({ kind: "error", code: "http.error", status: response.status }); }
    signal?.throwIfAborted();
    if (response.status === 404 && !body.code) throw new AdapterError("not-found");
    if (!response.ok) throw new AdapterOutcomeError({ kind: "error", code: body.code ?? "http.error", status: response.status });
    if (!body.success || !("data" in body)) throw new Error(`${operation}.invalid-response`);
    const reason = response.headers.get("X-Docklet-Host-Reason") as InventoryObservation["reason"];
    observed?.({ stale: !!reason, reason: reason || undefined, observedAt: response.headers.get("X-Docklet-Observed-At") ?? undefined });
    return body.data;
  }
  async function context(hostId: string, signal?: AbortSignal) {
    const [containers, projects] = await Promise.allSettled([transport.list("list-containers", hostId, "containers", undefined, signal), transport.list("list-projects", hostId, "projects", undefined, signal)]);
    // Hub maintenance fails the read instead of dropping the context's bind rows; other failures keep the fallback.
    for (const leg of [containers, projects]) if (leg.status === "rejected" && isMaintenanceError(leg.reason)) throw leg.reason;
    return { containers: containers.status === "fulfilled" ? containers.value.rows : [], projects: projects.status === "fulfilled" ? projects.value.rows : [] };
  }
  function supplement(volume: Volume, row: Schema["VolumeVolume"], hostId: string): Volume {
    for (const raw of row.containers ?? []) {
      const id = inventoryId(hostId, raw);
      if (!volume.consumers.some((c) => c.containerId === id)) volume.consumers.push(Object.assign({ containerId: id, containerName: raw, target: "", readOnly: false, running: false }, { runningUnavailable: true }));
    }
    return volume;
  }
  function map(row: Schema["VolumeVolume"], hostId: string, ctx: Awaited<ReturnType<typeof context>>, inventory: InventoryObservation): Volume {
    return supplement(mapVolumes([row], hostId, ctx.containers, ctx.projects, inventory)[0]!, row, hostId);
  }
  /** Bind rows come from container mounts; the agent's bind list adds kind, path-rule verdict and sizes. */
  async function bindList(hostId: string, signal?: AbortSignal): Promise<Map<string, BindInfo> | undefined> {
    const rows = await read("list-volume-binds", hostId, "binds", undefined, signal).catch((error) => {
      if (error instanceof CancelledError) throw error;
      return undefined;
    });
    return rows ? new Map(rows.map((b) => [b.source, {
      kind: b.kind, eligible: b.eligible, reason: b.reason || undefined,
      measuredSize: b.measuredSize, measuredAt: b.measuredAt, measuredPartial: b.measuredPartial,
      lastSnapshotSize: b.lastSnapshotSize, lastSnapshotAt: b.lastSnapshotAt,
    }])) : undefined;
  }
  function withBind(volume: Volume, binds: Map<string, BindInfo> | undefined): Volume {
    const bind = volume.kind === "bind" ? binds?.get(volume.source) : undefined;
    if (!bind) return volume;
    const size = bindSize(bind);
    return { ...volume, bind, fileBind: bind.kind === "file", sizeBytes: size?.bytes ?? 0, sizeUnavailable: !size };
  }
  const api: Pick<DockletAdapter, "listVolumes" | "getVolume"> = {
    async listVolumes(onPartial, parent) {
      const budget = inventoryReadBudget(parent);
      const recovery = inventoryRecovery();
      const check = begin();
      const scope = await budget(async () => { await requireCapability("listVolumes"); return hosts(); });
      check();
      const batches = new Map([...retained].filter(([id]) => scope.some((h) => h.id === id)));
      await Promise.all(scope.map(async (host) => {
        try {
          const { combined, observation } = await budget(async (signal) => {
            const [volumes, ctx, binds] = await Promise.all([transport.list("list-volumes", host.id, "volumes", undefined, signal), context(host.id, signal), bindList(host.id, signal)]);
            signal.throwIfAborted(); check();
            const rows = mapVolumes(volumes.rows, host.id, ctx.containers, ctx.projects, volumes.observation);
            const byName = new Map(volumes.rows.map((row) => [row.name, row]));
            const mapped = rows.map((v) => v.kind === "volume" ? supplement(v, byName.get(v.name)!, host.id) : withBind(v, binds));
            const combined = await decorate(host.id, mapped, signal);
            signal.throwIfAborted(); check();
            return { combined, observation: volumes.observation };
          });
          check();
          retained.set(host.id, combined); batches.set(host.id, combined);
          recovery.observed(host, observation);
          onState(host.id, observation);
        } catch (error) {
          parent?.throwIfAborted(); if (isMaintenanceError(error)) throw error; recovery.failed(host, error); check();
          const state = inventoryFailure(host.id, error);
          onState(host.id, state);
          const old = retained.get(host.id);
          if (old) batches.set(host.id, old.map((v) => ({ ...v, inventory: state })));
        }
        onPartial?.([...batches.values()].flat());
      }));
      check();
      return recovery.result([...batches.values()].flat());
    },
    async getVolume(id, signal) {
      const check = begin();
      await requireCapability("getVolume");
      const [hostId, name] = splitInventoryId(id);
      if (name.startsWith("bind:")) {
        const volume = (await api.listVolumes(undefined, signal)).find((v) => v.id === id);
        if (!volume) throw new AdapterOutcomeError({ kind: "error", code: "http.error", status: 404 });
        return { ...volume, backups: [] };
      }
      let inventory: InventoryObservation = { stale: false };
      const [row, ctx, sizes] = await Promise.all([
        read("get-volume", hostId, encodeURIComponent(name), (state) => { inventory = state; }, signal),
        context(hostId, signal),
        read("get-volume-sizes", hostId, "sizes", undefined, signal).catch(() => null),
      ]);
      check();
      signal?.throwIfAborted();
      const volume = map(row, hostId, ctx, inventory);
      onState(hostId, inventory);
      const size = sizes?.find((s) => s.name === name)?.size;
      if (typeof size === "number" && size >= 0) { volume.sizeBytes = size; volume.sizeUnavailable = false; }
      return { ...volume, scope: row.scope || undefined, backups: [] };
    },
  };
  return api;
}
