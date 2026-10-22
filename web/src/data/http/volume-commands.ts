// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 1e03c054f5d2c355759cb79cbedfaf97706d4be1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original volume mutation transport for issue #181.

import type { components } from "@/api/generated/schema";
import type { DockletAdapter } from "@/data/adapter";
import { AdapterError } from "@/data/api";
import { AdapterOutcomeError, unavailable, type CapabilityDescriptor, type ResourceRef } from "@/data/capability";
import type { CsrfClient } from "./csrf-client";
import { splitInventoryId } from "./inventory-mapping";
import { mapVolumes } from "./resource-mapping";

export interface VolumeTransportOptions {
  baseUrl: string;
  client: CsrfClient;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  changed: () => void;
}

export function createVolumeTransport({ baseUrl, client, capability }: VolumeTransportOptions) {
  async function require(operation: string, ref: ResourceRef) {
    const descriptor = await capability(operation, ref);
    if (!descriptor.available) throw unavailable(operation, ref, descriptor.disabledReasonCode);
  }
  async function request<T>(host: string, suffix: string, method = "GET", value?: object): Promise<T> {
    let response: Response;
    try {
      response = await client.fetch(`${baseUrl}/environments/${encodeURIComponent(host)}/volumes${suffix}`, { method, credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json", ...(value ? { "Content-Type": "application/json" } : {}) }, ...(value ? { body: JSON.stringify(value) } : {}) });
    } catch { throw new AdapterOutcomeError({ kind: "error", code: method === "GET" ? "http.error" : "volume.outcome-unknown" }); }
    let body: { success?: boolean; data: T; code?: string; detail?: string };
    try { body = await response.json(); } catch { throw new AdapterOutcomeError({ kind: "error", code: method === "GET" ? "http.error" : "volume.outcome-unknown", status: response.status }); }
    if (!response.ok) {
      // The current remove handler wraps Docker's conflict in a Huma 500 detail.
      if (body.code === "volume.in-use" || (response.status === 409 || response.status === 500) && /\bvolume (?:is )?in use\b/i.test(body.detail ?? "")) throw new AdapterError("in-use");
      if (response.status === 404 && !body.code) throw new AdapterError("not-found");
      throw new AdapterOutcomeError({ kind: "error", code: body.code ?? "http.error", status: response.status });
    }
    if (body.success !== true || !("data" in body)) throw new AdapterOutcomeError({ kind: "error", code: "volume.outcome-unknown" });
    return body.data;
  }
  return { require, request };
}

export function createVolumeCommands(options: VolumeTransportOptions): Pick<DockletAdapter, "createVolume" | "deleteVolume" | "renameVolume"> {
  const { require, request } = createVolumeTransport(options);
  const mapped = (row: components["schemas"]["VolumeVolume"], host: string) => mapVolumes([row], host, [], [], { stale: false })[0]!;
  return {
    async createVolume(input) {
      await require("createVolume", { kind: "host", id: input.hostId });
      try { return mapped(await request(input.hostId, "", "POST", { name: input.name, driver: "local", labels: input.labels }), input.hostId); }
      finally { options.changed(); }
    },
    async deleteVolume(id) {
      const [host, name] = splitInventoryId(id);
      if (name.startsWith("bind:")) throw new AdapterError("bind-not-deletable");
      await require("deleteVolume", { kind: "volume", id });
      try { await request(host, `/${encodeURIComponent(name)}?force=false`, "DELETE"); }
      finally { options.changed(); }
    },
    async renameVolume(id, name) {
      const [host, previous] = splitInventoryId(id);
      if (previous.startsWith("bind:")) throw new AdapterError("invalid-name");
      await require("renameVolume", { kind: "volume", id });
      try { return mapped(await request(host, `/${encodeURIComponent(previous)}/rename`, "POST", { name }), host); }
      finally { options.changed(); }
    },
  };
}
