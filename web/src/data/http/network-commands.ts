// Source: docklet-next/docs/design/network-management.md
// Revision: 2e2b3e5996afd98ee8edca0a8899947b62a2a339
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Ambiguous Direct/Edge proxy write failures for #209; Original capability-gated network command transport for issue #204.

import { isCancelledError } from "@tanstack/react-query";
import type { components } from "@/api/generated/schema";
import type { DockletAdapter } from "@/data/adapter";
import { AdapterError, type AdapterErrorCode } from "@/data/api";
import { AdapterOutcomeError, unavailable, type CapabilityDescriptor, type ResourceRef } from "@/data/capability";
import { draftFamilies, type NetworkDraft } from "@/domain/networks";
import type { CsrfClient } from "./csrf-client";
import { splitInventoryId } from "./inventory-mapping";
import { mapTopology } from "./resource-mapping";

type Schema = components["schemas"];
const ERRORS = new Set(["not-found", "exists", "invalid-network", "builtin-network", "compose-managed", "in-use", "network-mode", "already-connected", "not-connected", "invalid-ip", "overlap"]);

interface NetworkTransportOptions {
  baseUrl: string;
  client: CsrfClient;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  changed: () => void;
}

/** Empty bridge pools are omitted so Docker can choose them; disabled families never send pools. */
export function networkCreateRequest(input: NetworkDraft): Schema["NetworkCreateRequest"] {
  const families = draftFamilies(input);
  const config: Schema["NetworkIPAMConfig"][] = [];
  for (const [enabled, subnet, gateway, ipRange, aux] of [
    [families.ipv4, input.subnet, input.gateway, input.ipRange, input.auxAddresses],
    [families.ipv6, input.subnet6, input.gateway6, input.ipRange6, input.auxAddresses6],
  ] as const) {
    if (!enabled || !(subnet?.trim() || gateway?.trim() || ipRange?.trim() || aux?.length)) continue;
    config.push({ subnet: subnet?.trim() || undefined, gateway: gateway?.trim() || undefined, ipRange: ipRange?.trim() || undefined, auxAddress: Object.fromEntries((aux ?? []).map((a) => [a.name.trim(), a.ip.trim()])) });
  }
  const { internal } = input;
  return { name: input.name.trim(), options: { driver: input.driver, internal, enableIPv4: families.ipv4, enableIPv6: families.ipv6, labels: input.labels ?? {}, options: { ...input.options, ...(input.driver !== "bridge" ? { parent: input.parent.trim() } : {}) }, ipam: { config } } };
}

export function createNetworkCommands(options: NetworkTransportOptions): Pick<DockletAdapter, "createNetwork" | "removeNetwork" | "connectNetwork" | "disconnectNetwork"> {
  const { baseUrl, client, capability } = options;
  async function require(operation: string, ref: ResourceRef) {
    const descriptor = await capability(operation, ref);
    if (!descriptor.available) throw unavailable(operation, ref, descriptor.disabledReasonCode);
  }
  async function request<T>(host: string, suffix: string, method = "GET", value?: object): Promise<T> {
    const uncertain = () => new AdapterOutcomeError({ kind: "error", code: method === "GET" ? "http.error" : "network.outcome-unknown" });
    let response: Response;
    try {
      response = await client.fetch(`${baseUrl}/environments/${encodeURIComponent(host)}/networks${suffix}`, { method, credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json", ...(value ? { "Content-Type": "application/json" } : {}) }, ...(value ? { body: JSON.stringify(value) } : {}) });
    } catch (error) { if (isCancelledError(error)) throw error; throw uncertain(); }
    let body: { success?: boolean; data: T; code?: string };
    try { body = await response.json(); } catch (error) { if (isCancelledError(error)) throw error; throw uncertain(); }
    if (!response.ok) {
      if (method !== "GET" && (response.status === 502 || response.status === 504) && body?.code === "agent.unavailable") {
        throw new AdapterOutcomeError({ kind: "error", code: "network.outcome-unknown", status: response.status });
      }
      const code = typeof body?.code === "string" ? body.code : "http.error";
      const key = code.startsWith("network.") ? code.slice("network.".length) : "";
      if (ERRORS.has(key)) throw new AdapterError(key as AdapterErrorCode);
      throw new AdapterOutcomeError({ kind: "error", code, status: response.status });
    }
    if (!body || body.success !== true || !("data" in body)) throw uncertain();
    return body.data;
  }
  async function target(operation: string, networkId: string, containerId: string) {
    const [host, network] = splitInventoryId(networkId);
    await require(operation, { kind: "network", id: networkId });
    const [containerHost, container] = splitInventoryId(containerId);
    if (containerHost !== host) throw new AdapterError("host-mismatch");
    return { host, network, container };
  }
  return {
    async createNetwork(input) {
      await require("createNetwork", { kind: "host", id: input.hostId });
      try {
        const created = await request<Schema["NetworkCreateResponse"]>(input.hostId, "", "POST", networkCreateRequest(input));
        if (!created?.id) throw new AdapterOutcomeError({ kind: "error", code: "network.outcome-unknown" });
        try {
          const row = await request<Schema["NetworkInspect"]>(input.hostId, `/${encodeURIComponent(created.id)}`);
          return mapTopology([row], input.hostId, [], [], { stale: false }).networks[0]!;
        } catch (error) { if (isCancelledError(error)) throw error; throw new AdapterOutcomeError({ kind: "error", code: "network.outcome-unknown" }); }
      } finally { options.changed(); }
    },
    async removeNetwork(networkId) {
      const [host, id] = splitInventoryId(networkId);
      await require("removeNetwork", { kind: "network", id: networkId });
      try { await request<Schema["BaseMessageResponse"]>(host, `/${encodeURIComponent(id)}`, "DELETE"); }
      finally { options.changed(); }
    },
    async connectNetwork(input) {
      const { host, network, container } = await target("connectNetwork", input.networkId, input.containerId);
      const body: Schema["NetworkConnectContainerRequest"] = { containerId: container, aliases: input.aliases, ipv4Address: input.ip?.trim() || undefined, ipv6Address: input.ip6?.trim() || undefined };
      try {
        await request<Schema["BaseMessageResponse"]>(host, `/${encodeURIComponent(network)}/connect`, "POST", body);
        // The acknowledgement confirms membership; automatic addresses arrive through topology refresh.
        return { containerId: input.containerId, networkId: input.networkId, ip: body.ipv4Address, ip6: body.ipv6Address, staticIp: body.ipv4Address ? true : undefined, staticIp6: body.ipv6Address ? true : undefined, aliases: input.aliases };
      } finally { options.changed(); }
    },
    async disconnectNetwork(input) {
      const { host, network, container } = await target("disconnectNetwork", input.networkId, input.containerId);
      const body: Schema["NetworkDisconnectContainerRequest"] = { containerId: container };
      try { await request<Schema["BaseMessageResponse"]>(host, `/${encodeURIComponent(network)}/disconnect`, "POST", body); }
      finally { options.changed(); }
    },
  };
}
