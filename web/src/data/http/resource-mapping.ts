// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original Docker resource view-model mappings. Hub treated as a system workload like the agent.

import { systemRole } from "@/domain/system";
import { mapImageObservation } from "./image-mapping";
import type { components } from "@/api/generated/schema";
import type { DockerImage } from "@/domain/images";
import type { InventoryObservation, Topology, TopologyNetwork } from "@/domain/model";
import { bindKey, type Volume, type VolumeConsumer } from "@/domain/volumes";
import { inventoryId, mapContainer, type ContainerRow, type ProjectRow } from "./inventory-mapping";

import { composeProjectOwner } from "./project-ownership";
import { representableId, rowCollector, type RowCollector } from "./row-isolation";

// A consumer container this reader tries to map and cannot is counted once under the reader's own kind:
// partial context scans never publish the host's containers count (docs/design/inventory-row-isolation.md).

type Schema = components["schemas"];
export function mapImage(row: Schema["ImageSummary"], hostId: string, containers: ContainerRow[], projects: ProjectRow[], inventory: InventoryObservation, unreadable: RowCollector = rowCollector("images")): DockerImage {
  if (!representableId(row?.id) || typeof row.size !== "number" || !Number.isFinite(row.created)) throw new Error("inventory.invalid-image");
  const tags = (row.repoTags ?? []).filter((tag) => tag !== "<none>:<none>");
  const reference = tags[0];
  const colon = reference?.lastIndexOf(":") ?? -1;
  const repository = reference ? reference.slice(0, colon > reference.lastIndexOf("/") ? colon : undefined) : row.repo === "<none>" ? "" : row.repo;
  // Consumers reference other containers: each one that cannot be mapped is left out and counted once (#600).
  const consumers = unreadable.each(containers.filter((c) => c?.imageId === row.id), (raw) => {
    const c = mapContainer(raw, hostId, projects, inventory);
    return { containerId: c.id, name: c.name, hostId, stackId: c.stackId, stackName: c.stackName, serviceName: c.serviceName, state: c.state, ...(systemRole(raw.labels) !== undefined ? { system: true } : {}) };
  }, containerName, containerKey);
  // Hidden system consumers still protect shared images from appearing unused.
  const usedBy = row.usedBy ?? [];
  if (!Array.isArray(usedBy)) throw new Error("inventory.invalid-image");
  unreadable.each(usedBy.filter((usage) => usage?.type === undefined || usage.type === "container"), (usage) => {
    if (usage?.type !== "container" || !representableId(usage.id) || (usage.name != null && typeof usage.name !== "string")) throw new Error("inventory.invalid-consumer");
    const containerId = inventoryId(hostId, usage.id);
    if (!consumers.some((c) => c.containerId === containerId)) consumers.push({ containerId, name: usage.name || usage.id, hostId, stackId: undefined, stackName: undefined, serviceName: undefined, state: "unknown", system: true });
  }, (usage) => usage?.name, (usage) => typeof usage?.id === "string" ? `container:${usage.id}` : undefined);
  return { ...mapImageObservation(row), id: inventoryId(hostId, row.id), hostId, repository, tag: reference ? colon > reference.lastIndexOf("/") ? reference.slice(colon + 1) : "" : null, aliases: tags.slice(1), digest: row.repoDigests?.[0]?.split("@")[1] ?? row.id,
    sizeMb: row.size / 1024 ** 2, createdAt: new Date(row.created * 1000).toISOString(), architecture: "", os: "", layers: 0, metadataUnavailable: true, consumers, inventory };
}

/** One physical network row counts once, whether its summary, detail or projection failed. */
export function networkKey(row: { id?: unknown } | null | undefined): string | undefined {
  return typeof row?.id === "string" ? `network:${row.id}` : undefined;
}

function containerKey(row: { id?: unknown } | null | undefined): string | undefined {
  return typeof row?.id === "string" ? `container:${row.id}` : undefined;
}

function containerName(row: ContainerRow | null | undefined): unknown {
  return row?.names?.[0]?.replace(/^\//, "");
}

function owner(labels: Record<string, string> | null | undefined, hostId: string, projects: ProjectRow[]) {
  const project = composeProjectOwner(projects, labels?.["com.docker.compose.project"]);
  return project ? { stackId: inventoryId(hostId, project.id), stackName: project.name } : {};
}
export function mapVolumes(rows: Schema["VolumeVolume"][], hostId: string, containers: ContainerRow[], projects: ProjectRow[], inventory: InventoryObservation, unreadable: RowCollector = rowCollector("volumes")): Volume[] {
  const usage = new Map<string, VolumeConsumer[]>();
  const binds = new Map<string, Volume>();
  // A container or mount that cannot be mapped drops only its own consumer entries (#600).
  for (const raw of containers) {
    unreadable.one(raw, (raw) => {
      if (systemRole(raw.labels) !== undefined) return;
      const c = mapContainer(raw, hostId, projects, inventory);
      for (const m of raw.mounts ?? []) {
        if (m?.type !== "volume" && m?.type !== "bind") continue;
        const name = m.type === "volume" ? m.name : m.source;
        if (!name) continue;
        const key = `${m.type}:${name}`;
        const consumers = usage.get(key) ?? [];
        consumers.push({ containerId: c.id, containerName: c.name, stackId: c.stackId, stackName: c.stackName, service: c.serviceName, target: m.destination, readOnly: !m.rw, running: raw.state === "running" });
        usage.set(key, consumers);
        if (m.type === "bind") binds.set(key, { id: inventoryId(hostId, bindKey(name)), name, hostId, kind: "bind", source: name, sizeBytes: 0, sizeUnavailable: true, createdAt: "", consumers, labels: {}, inventory });
      }
    }, containerName, containerKey);
  }
  return [...unreadable.each(rows.filter((r) => systemRole(r?.labels) === undefined), (r): Volume => {
    if (!representableId(r?.name) || typeof r.driver !== "string" || !r.driver) throw new Error("inventory.invalid-volume");
    const size = r.usageData?.Size ?? (r.size > 0 ? r.size : undefined);
    return { id: inventoryId(hostId, r.name), hostId, name: r.name, kind: "volume", driver: r.driver, source: r.mountpoint, composeName: r.labels?.["com.docker.compose.volume"], ...owner(r.labels, hostId, projects), sizeBytes: typeof size === "number" && size >= 0 ? size : 0, sizeUnavailable: size == null || size < 0, createdAt: r.createdAt || "", labels: r.labels ?? {}, consumers: usage.get(`volume:${r.name}`) ?? [], inventory };
  }, (r) => r?.name), ...binds.values()];
}

/** Moby's EndpointIPAMConfig is passed through the DTO's unknown ipamConfig field. */
function fixedAddress(config: unknown, key: "IPv4Address" | "IPv6Address"): string | undefined {
  if (!config || typeof config !== "object" || !(key in config)) return undefined;
  const value = (config as Record<string, unknown>)[key];
  return typeof value === "string" && value ? value : undefined;
}

export function mapTopology(rows: (Schema["NetworkInspect"] | Schema["NetworkSummary"])[], hostId: string, containers: ContainerRow[], projects: ProjectRow[], inventory: InventoryObservation, summaries: Pick<Schema["NetworkSummary"], "id" | "isDefault">[] = [], unavailable = new Map<string, InventoryObservation>(), unreadable: RowCollector = rowCollector("networks")): Topology {
  const defaults = new Map(summaries.flatMap((n) => n ? [[n.id, n.isDefault] as const] : []));
  // A network row that cannot be mapped is left out; a container that cannot be placed loses only its own attachments (#600).
  const networks: TopologyNetwork[] = unreadable.each(rows.filter((r) => systemRole(r?.labels) === undefined), (r) => {
    if (!representableId(r?.id) || typeof r.name !== "string" || !r.name || typeof r.driver !== "string" || !r.driver) throw new Error("inventory.invalid-network");
    const { internal } = "internal" in r ? r : { internal: false };
    const ip4 = r.ipam.config?.find((c) => c.subnet && !c.subnet.includes(":"));
    const ip6 = r.ipam.config?.find((c) => c.subnet?.includes(":"));
    return { id: inventoryId(hostId, r.id), hostId, name: r.name, driver: r.driver, internal, labels: r.labels ?? {}, options: r.options ?? {}, auxAddresses: Object.entries(ip4?.auxAddress ?? {}).map(([name, ip]) => ({ name, ip })), auxAddresses6: Object.entries(ip6?.auxAddress ?? {}).map(([name, ip]) => ({ name, ip })), builtIn: defaults.get(r.id) === true, ipv4: "enableIPv4" in r ? r.enableIPv4 : undefined, ipv6: "enableIPv6" in r ? r.enableIPv6 : undefined, subnet: ip4?.subnet, gateway: ip4?.gateway, ipRange: ip4?.ipRange, subnetConfigured: !!ip4?.subnet, subnet6: ip6?.subnet, gateway6: ip6?.gateway, ipRange6: ip6?.ipRange, subnet6Configured: !!ip6?.subnet, parent: r.options?.parent, ...owner(r.labels, hostId, projects), inventory: unavailable.get(r.id) ?? inventory, ...(unavailable.has(r.id) ? { detailUnavailable: true } : {}) };
  }, (r) => r?.name, networkKey);
  // Dependency targets are context rows: only representable identities can be matched (#600).
  const users = containers.filter((c) => !!c && representableId(c.id) && systemRole(c.labels) === undefined);
  const attachments: Topology["attachments"] = [];
  const dependencies: Topology["dependencies"] = [];
  for (const c of containers) unreadable.one(c, (c) => {
    if (systemRole(c.labels) !== undefined) return;
    const containerId = inventoryId(hostId, c.id);
    const endpoints = c.networkSettings?.networks;
    // Each attachment is a reference entry of its own: one that cannot be read drops only itself (#600).
    for (const n of networks) unreadable.one(n, (n) => {
      const endpoint = endpoints?.[n.name];
      const detail = rows.find((r) => typeof r?.id === "string" && `${hostId}~${r.id}` === n.id);
      const raw = detail && "containers" in detail ? detail.containers?.[c.id] : undefined;
      if (!endpoint && !raw) return;
      const fixed4 = fixedAddress(endpoint?.ipamConfig, "IPv4Address");
      const fixed6 = fixedAddress(endpoint?.ipamConfig, "IPv6Address");
      attachments.push({ containerId, networkId: n.id, ip: endpoint?.ipAddress || raw?.IPv4Address?.split("/")[0] || fixed4, ip6: endpoint?.globalIPv6Address || raw?.IPv6Address?.split("/")[0] || fixed6, staticIp: fixed4 ? true : undefined, staticIp6: fixed6 ? true : undefined, aliases: endpoint?.aliases ?? undefined });
    }, (n) => n.name, (n) => `attachment:${n.id}:${c.id}`);
    if (!c.labels?.["com.docker.compose.project"]) return;
    for (const entry of (c.labels?.["com.docker.compose.depends_on"] ?? "").split(",").filter(Boolean)) {
      const service = entry.split(":")[0];
      for (const target of users.filter((candidate) => candidate.id !== c.id && candidate.labels?.["com.docker.compose.project"] === c.labels?.["com.docker.compose.project"] && candidate.labels?.["com.docker.compose.service"] === service)) dependencies.push({ from: containerId, to: inventoryId(hostId, target.id) });
    }
  }, containerName, containerKey);
  return { networks, attachments, dependencies };
}
