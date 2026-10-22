// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 1e03c054f5d2c355759cb79cbedfaf97706d4be1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original live volume mapping and regression coverage for issue #181.

import { CancelledError } from "@tanstack/react-query";

import type { components, operations } from "@/api/generated/schema";
import { AdapterError } from "@/data/api";
import type { DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import type { Host, InventoryObservation } from "@/domain/model";
import type { Volume } from "@/domain/volumes";
import { inventoryId, splitInventoryId } from "./inventory-mapping";
import { createInventoryTransport, inventoryFailure } from "./inventory-transport";
import { mapVolumes } from "./resource-mapping";

type Read = "get-volume" | "get-volume-usage" | "get-volume-sizes";
type Body<I extends Read> = operations[I]["responses"][200]["content"]["application/json"];
type Schema = components["schemas"];

export function createVolumeReads(baseUrl: string, fetcher: typeof fetch, hosts: () => Promise<Host[]>, requireCapability: (operation: string) => Promise<void>, onState: (id: string, state: InventoryObservation) => void, generation: () => number): Pick<DockletAdapter, "listVolumes" | "getVolume"> {
  const transport = createInventoryTransport(baseUrl, fetcher, onState);
  const retained = new Map<string, Volume[]>();
  let owner = generation();
  function begin() {
    const version = generation();
    if (owner !== version) { retained.clear(); owner = version; }
    return () => { if (generation() !== version) throw new CancelledError(); };
  }
  async function read<I extends Read>(operation: I, hostId: string, suffix: string, observed?: (state: InventoryObservation) => void): Promise<Body<I>["data"]> {
    const response = await fetcher(`${baseUrl}/environments/${encodeURIComponent(hostId)}/volumes/${suffix}`, { credentials: "same-origin", headers: { Accept: "application/json" } });
    let body: Body<I> & { code?: string };
    try { body = await response.json(); } catch { throw new AdapterOutcomeError({ kind: "error", code: "http.error", status: response.status }); }
    if (response.status === 404 && !body.code) throw new AdapterError("not-found");
    if (!response.ok) throw new AdapterOutcomeError({ kind: "error", code: body.code ?? "http.error", status: response.status });
    if (!body.success || !("data" in body)) throw new Error(`${operation}.invalid-response`);
    const reason = response.headers.get("X-Docklet-Host-Reason") as InventoryObservation["reason"];
    observed?.({ stale: !!reason, reason: reason || undefined, observedAt: response.headers.get("X-Docklet-Observed-At") ?? undefined });
    return body.data;
  }
  async function context(hostId: string) {
    const [containers, projects] = await Promise.allSettled([transport.list("list-containers", hostId, "containers"), transport.list("list-projects", hostId, "projects")]);
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
  const api: Pick<DockletAdapter, "listVolumes" | "getVolume"> = {
    async listVolumes(onPartial) {
      const check = begin();
      await requireCapability("listVolumes");
      const scope = await hosts();
      check();
      const batches = new Map([...retained].filter(([id]) => scope.some((h) => h.id === id)));
      await Promise.all(scope.map(async (host) => {
        try {
          const [volumes, ctx] = await Promise.all([transport.list("list-volumes", host.id, "volumes"), context(host.id)]);
          check();
          const rows = mapVolumes(volumes.rows, host.id, ctx.containers, ctx.projects, volumes.observation);
          const byName = new Map(volumes.rows.map((row) => [row.name, row]));
          const combined = rows.map((v) => v.kind === "volume" ? supplement(v, byName.get(v.name)!, host.id) : v);
          retained.set(host.id, combined); batches.set(host.id, combined);
          onState(host.id, volumes.observation);
        } catch (error) {
          check();
          const state = inventoryFailure(host.id, error);
          onState(host.id, state);
          const old = retained.get(host.id);
          if (old) batches.set(host.id, old.map((v) => ({ ...v, inventory: state })));
        }
        onPartial?.([...batches.values()].flat());
      }));
      check();
      return [...batches.values()].flat();
    },
    async getVolume(id) {
      const check = begin();
      await requireCapability("getVolume");
      const [hostId, name] = splitInventoryId(id);
      if (name.startsWith("bind:")) {
        const volume = (await api.listVolumes()).find((v) => v.id === id);
        if (!volume) throw new AdapterOutcomeError({ kind: "error", code: "http.error", status: 404 });
        return { ...volume, backups: [] };
      }
      let inventory: InventoryObservation = { stale: false };
      const [row, usage, ctx, sizes] = await Promise.all([
        read("get-volume", hostId, encodeURIComponent(name), (state) => { inventory = state; }),
        read("get-volume-usage", hostId, `${encodeURIComponent(name)}/usage`),
        context(hostId),
        read("get-volume-sizes", hostId, "sizes").catch(() => null),
      ]);
      check();
      const volume = map({ ...row, containers: [...new Set([...(row.containers ?? []), ...(usage.containers ?? [])])] }, hostId, ctx, inventory);
      onState(hostId, inventory);
      const size = sizes?.find((s) => s.name === name)?.size;
      if (typeof size === "number" && size >= 0) { volume.sizeBytes = size; volume.sizeUnavailable = false; }
      return { ...volume, scope: row.scope || undefined, backups: [] };
    },
  };
  return api;
}
