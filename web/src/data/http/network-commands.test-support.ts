// Source: docklet-next/docs/design/network-management.md
// Revision: 2e2b3e5996afd98ee8edca0a8899947b62a2a339
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original Direct/Edge fixtures using the recorded network handler envelopes.

import { readFileSync } from "node:fs";
import { vi } from "vitest";
import type { components } from "@/api/generated/schema";
import type { NetworkDraft } from "@/domain/networks";
import { createHttpAdapter } from "./http-adapter";

type Schema = components["schemas"];
export const recordedNetworkResponses = JSON.parse(readFileSync("../backend/api/testdata/network-write-responses.json", "utf8")) as Record<string, { status?: number; code?: string; data?: unknown }>;
export const networkDraft = (patch: Partial<NetworkDraft> = {}): NetworkDraft => ({ hostId: "host", name: "example_net", driver: "bridge", subnet: "192.0.2.0/24", gateway: "192.0.2.1", ipRange: "192.0.2.128/25", parent: "", internal: false, ...patch });
export const networkDto: Schema["NetworkInspect"] = { id: "net1", name: "example_net", driver: "bridge", scope: "local", created: "2026-10-24T00:00:00Z", ipam: { config: [{ subnet: "192.0.2.0/24", gateway: "192.0.2.1" }] }, configFrom: { Network: "" }, containers: {}, options: {}, labels: {}, containersList: [], enableIPv4: true, enableIPv6: false, internal: false, attachable: false, ingress: false, configOnly: false };
export const networkContainer: Schema["ContainerSummary"] = { id: "container1", names: ["/example-container"], image: "example:1.0.0", imageId: "sha256:example", command: "serve", created: 0, ports: [], labels: {}, state: "running", status: "Up", hostConfig: {}, mounts: [], networkSettings: { networks: {} }, autoUpdateEnabled: false };
const operations = ["createNetwork", "removeNetwork", "connectNetwork", "disconnectNetwork", "listHosts", "listContainers", "listStacks", "getTopology"];

export function networkFixture(edge = false) {
  const state = { deniedHosts: new Set<string>(), response: undefined as { body: unknown; status: number } | undefined, failTransport: false, malformed: false, inspectFails: false, hosts: ["host", "other"], networks: [structuredClone(networkDto)], containers: [] as Schema["ContainerSummary"][], calls: [] as { path: string; method: string; body: unknown; credentials?: RequestCredentials; cache?: RequestCache }[] };
  const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "X-CSRF-Token": "example".padEnd(43, "x"), "Content-Type": "application/json" } });
  const data = (value: unknown, paginated = false) => json({ success: true, data: value, ...(paginated ? { pagination: { totalItems: Array.isArray(value) ? value.length : 0 } } : {}) });
  const source = { close: vi.fn(), onopen: null, onmessage: null, onerror: null };
  const fetcher = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname.endsWith("/capabilities")) {
      const id = url.searchParams.get("id") ?? "0";
      const host = id.split("~")[0] ?? "";
      return data(operations.map((operationId) => ({ operationId, available: !state.deniedHosts.has(host), disabledReasonCode: "capability.unavailable", resourceRef: { kind: url.searchParams.get("kind"), id } })));
    }
    if (url.pathname.endsWith("/environments")) return data(state.hosts.map((id) => ({ id, name: id, apiUrl: "https://agent.example.com", isEdge: edge, status: "online", enabled: true })), true);
    if (url.pathname.endsWith("/settings")) return data({ effectiveTimeZone: "UTC", settings: [] });
    if (url.pathname.endsWith("/projects")) return data([], true);
    if (url.pathname.endsWith("/containers")) return data(state.containers, true);
    const method = init?.method ?? "GET";
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    state.calls.push({ path: url.pathname, method, body, credentials: init?.credentials, cache: init?.cache });
    if (method !== "GET") {
      if (state.failTransport) throw new TypeError("connection lost");
      if (state.malformed) return json({ success: true });
      if (state.response) return json(state.response.body, state.response.status);
      if (method === "DELETE") {
        // Delete/create are not in the recording; these typed envelopes match NetworkHandler and generated DTOs.
        const message: Schema["BaseMessageResponse"] = { message: "Network removed successfully" };
        return data(message);
      }
      if (url.pathname.endsWith("/connect")) return json(recordedNetworkResponses.connect);
      if (url.pathname.endsWith("/disconnect")) return json(recordedNetworkResponses.disconnect);
      const request = body as Schema["NetworkCreateRequest"];
      const { internal, enableIPv4, enableIPv6, driver, labels, options, ipam } = request.options;
      state.networks.push({ ...structuredClone(networkDto), id: "created", name: request.name, internal: internal ?? false, enableIPv4: enableIPv4 ?? true, enableIPv6: enableIPv6 ?? false, driver: driver ?? "bridge", labels: labels ?? {}, options: options ?? {}, ipam: ipam ?? {} });
      const result: Schema["NetworkCreateResponse"] = { id: "created" };
      return data(result);
    }
    if (url.pathname.endsWith("/networks")) return data(state.networks.map((n): Schema["NetworkSummary"] => ({ id: n.id, name: n.name, created: n.created, driver: n.driver, scope: n.scope, ipam: n.ipam, labels: n.labels, options: n.options, isDefault: false, inUse: false })), true);
    if (state.inspectFails) throw new TypeError("inspect unavailable");
    const found = state.networks.find((n) => url.pathname.endsWith(`/networks/${encodeURIComponent(n.id)}`));
    return found ? data(found) : json(recordedNetworkResponses["missing-network"], 404);
  });
  return { state, json, data, fetcher, source, adapter: createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => source }) };
}
