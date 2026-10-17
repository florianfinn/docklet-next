// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Synthetic two-host Docker inventory fixtures.

import { vi } from "vitest";
import type { components } from "@/api/generated/schema";
import type { ContainerRow, ProjectRow } from "./inventory-mapping";
import type { MetricsSocket } from "./metrics";
import { createHttpAdapter } from "./http-adapter";

export const at = "2026-10-17T12:00:00Z";
const project: ProjectRow = { id: "project", name: "app", status: "running", isArchived: false, runningCount: 2, serviceCount: 2, path: "/srv/projects/app", createdAt: at, updatedAt: at, tags: [], runtimeServices: [] };
const container: ContainerRow = { id: "web", names: ["/web"], image: "example:1.0.0", imageId: "sha256:user", command: "serve", created: 0, ports: [{ privatePort: 80, publicPort: 8080, type: "tcp" }], labels: { "com.docker.compose.project": "app", "com.docker.compose.service": "web", "com.docker.compose.depends_on": "db:service_healthy:true" }, state: "running", status: "Up (healthy)", hostConfig: {}, networkSettings: { networks: { front: { networkId: "net", ipAddress: "192.0.2.2", globalIPv6Address: "2001:db8::2" } } }, mounts: [{ type: "volume", name: "data", source: "/srv/data", destination: "/data", driver: "local", mode: "rw", rw: true, propagation: "" }], resourceSample: { cpuPercent: 2.5, memoryUsageBytes: 104857600, memoryLimitBytes: 1073741824, sampleTime: at }, autoUpdateEnabled: false };
const image: components["schemas"]["ImageSummary"] = { id: "sha256:user", repo: "example", tag: "1.0.0", repoTags: ["example:1.0.0", "example:stable"], repoDigests: ["example@sha256:digest"], created: 1700000000, size: 104857600, virtualSize: 0, labels: {}, inUse: true, usedBy: [] };
const volume: components["schemas"]["VolumeVolume"] = { id: "data", name: "data", driver: "local", mountpoint: "/srv/data", scope: "local", createdAt: at, labels: { "com.docker.compose.project": "app", "com.docker.compose.volume": "data" }, options: {}, inUse: true, containers: ["web"], size: 0 };
const network: components["schemas"]["NetworkInspect"] = { id: "net", name: "front", driver: "bridge", scope: "local", created: at, ipam: { driver: "default", options: {}, config: [{ subnet: "192.0.2.0/24", gateway: "192.0.2.1" }] }, configFrom: { Network: "" }, containers: {}, options: {}, labels: { "com.docker.compose.project": "app" }, containersList: [], enableIPv4: true, enableIPv6: true, internal: true, attachable: false, ingress: false, configOnly: false };

export function resourceFixture() {
  const state = { reason: "", observedAt: at, status: "online", malformed: false, noSnapshot: false, bind: false, networks: [network], failureHost: "second", failure: "", hostIds: ["first", "second"] };
  const sockets = new Map<string, MetricsSocket>();
  const webSocket = vi.fn((url: string): MetricsSocket => {
    const socket: MetricsSocket = { onmessage: null, onclose: null, onerror: null, close: vi.fn() };
    sockets.set(url.includes("first") ? "first" : "second", socket);
    return socket;
  });
  const json = (data: unknown, paginated = false, observed = false, status = 200) => new Response(JSON.stringify(status === 200 ? { success: true, data, ...(paginated ? { pagination: { totalItems: Array.isArray(data) ? data.length : 0 } } : {}) } : { success: false, code: data }), { status, headers: { "Content-Type": "application/json", ...(observed ? { "X-Docklet-Observed-At": state.observedAt, ...(state.reason ? { "X-Docklet-Host-Reason": state.reason } : {}) } : {}) } });
  const fetcher = vi.fn(async (input: string | URL | Request) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname === "/api/capabilities") return json(["listHosts", "listStacks", "listContainers", "listImages", "listVolumes", "getVolume", "getTopology", "subscribeMetrics"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" } })));
    if (url.pathname === "/api/environments") return json(state.hostIds.map((id) => ({ id, name: id, apiUrl: "http://agent.example.com", enabled: true, status: state.status, isEdge: false })), true);
    if (url.pathname.endsWith("/settings")) return json({ effectiveTimeZone: "UTC", settings: [] });
    if (state.noSnapshot && url.pathname.includes("/environments/")) return json(state.reason || "agent.unavailable", false, false, 502);
    if (url.pathname.startsWith(`/api/environments/${state.failureHost}/`)) {
      if (state.failure === "http") return json("docker.error", false, false, 500);
      if (state.failure === "mapping") return json([{ id: "malformed" }], true, true);
    }
    if (url.pathname.endsWith("/projects")) return json([project], true, true);
    if (url.pathname.endsWith("/containers")) return json([{ ...container, mounts: state.bind ? [{ type: "bind", name: "", source: "/srv/example~data", destination: "/data", rw: true }] : container.mounts }, { ...container, id: "db", names: ["/db"], state: "exited", labels: { "com.docker.compose.project": "app", "com.docker.compose.service": "db" }, ports: [], mounts: [] }], true, true);
    if (url.pathname.endsWith("/images")) return json(state.malformed ? [{ id: "bad" }] : [image, { ...image, id: "unused", repoTags: ["unused:1"], usedBy: [], inUse: false }, { ...image, id: "dangling", repoTags: [], usedBy: [], inUse: false }], true, true);
    if (url.pathname.endsWith("/volumes")) return json([volume], true, true);
    if (url.pathname.endsWith("/networks")) return json(state.networks, true, true);
    const detail = state.networks.find((n) => url.pathname.endsWith(`/networks/${n.id}`));
    if (detail) return json(detail, false, true);
    return json("not-found", false, false, 404);
  });
  const source = { close() {}, onopen: null as ((event: Event) => void) | null, onmessage: null as ((event: MessageEvent) => void) | null, onerror: null as ((event: Event) => void) | null };
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch, webSocket, eventSource: () => source });
  const sample = (hostId: string, cpu = 25) => sockets.get(hostId)?.onmessage?.({ data: JSON.stringify({ cpuUsage: cpu, memoryUsage: 1073741824, memoryTotal: 4294967296, diskUsage: 2147483648, diskTotal: 8589934592, cpuCount: 4 }) } as MessageEvent);
  return { adapter, state, fetcher, sockets, webSocket, sample, source };
}
