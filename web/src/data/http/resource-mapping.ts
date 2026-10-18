// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original Docker resource view-model mappings.

import type { components } from "@/api/generated/schema";
import type { DockerImage } from "@/domain/images";
import type { InventoryObservation, Topology, TopologyNetwork } from "@/domain/model";
import type { Volume, VolumeConsumer } from "@/domain/volumes";
import { inventoryId, mapContainer, type ContainerRow, type ProjectRow } from "./inventory-mapping";

type Schema = components["schemas"];
export function mapImage(row: Schema["ImageSummary"], hostId: string, containers: ContainerRow[], projects: ProjectRow[], inventory: InventoryObservation): DockerImage {
  if (!row?.id || typeof row.size !== "number" || !Number.isFinite(row.created)) throw new Error("inventory.invalid-image");
  const tags = (row.repoTags ?? []).filter((tag) => tag !== "<none>:<none>");
  const reference = tags[0];
  const colon = reference?.lastIndexOf(":") ?? -1;
  const repository = reference ? reference.slice(0, colon > reference.lastIndexOf("/") ? colon : undefined) : row.repo === "<none>" ? "" : row.repo;
  const consumers = containers.filter((c) => c.imageId === row.id).map((raw) => {
    const c = mapContainer(raw, hostId, projects, inventory);
    return { containerId: c.id, name: c.name, hostId, stackId: c.stackId, stackName: c.stackName, serviceName: c.serviceName, state: c.state, ...(raw.labels?.["io.docklet.system"] === "agent" ? { system: true } : {}) };
  });
  // Hidden system consumers still protect shared images from appearing unused.
  for (const usage of row.usedBy ?? []) {
    const id = usage.id;
    if (usage.type === "container" && id && !consumers.some((c) => c.containerId === inventoryId(hostId, id))) consumers.push({ containerId: inventoryId(hostId, id), name: usage.name ?? id, hostId, stackId: undefined, stackName: undefined, serviceName: undefined, state: "unknown", system: true });
  }
  return { id: inventoryId(hostId, row.id), hostId, repository, tag: reference ? colon > reference.lastIndexOf("/") ? reference.slice(colon + 1) : "" : null, aliases: tags.slice(1), digest: row.repoDigests?.[0]?.split("@")[1] ?? row.id,
    sizeMb: row.size / 1024 ** 2, createdAt: new Date(row.created * 1000).toISOString(), architecture: "", os: "", layers: 0, metadataUnavailable: true, consumers, inventory };
}

function owner(labels: Record<string, string> | null | undefined, hostId: string, projects: ProjectRow[]) {
  const project = projects.find((p) => p.name === labels?.["com.docker.compose.project"]);
  return project ? { stackId: inventoryId(hostId, project.id), stackName: project.name } : {};
}
export function mapVolumes(rows: Schema["VolumeVolume"][], hostId: string, containers: ContainerRow[], projects: ProjectRow[], inventory: InventoryObservation): Volume[] {
  const usage = new Map<string, VolumeConsumer[]>();
  const binds = new Map<string, Volume>();
  for (const raw of containers) {
    if (raw.labels?.["io.docklet.system"] === "agent") continue;
    const c = mapContainer(raw, hostId, projects, inventory);
    for (const m of raw.mounts ?? []) {
      if (m.type !== "volume" && m.type !== "bind") continue;
      const name = m.type === "volume" ? m.name : m.source;
      if (!name) throw new Error("inventory.invalid-mount");
      const key = `${m.type}:${name}`;
      const consumers = usage.get(key) ?? [];
      consumers.push({ containerId: c.id, containerName: c.name, stackId: c.stackId, stackName: c.stackName, service: c.serviceName, target: m.destination, readOnly: !m.rw, running: raw.state === "running" });
      usage.set(key, consumers);
      if (m.type === "bind") binds.set(key, { id: inventoryId(hostId, `bind:${Array.from(new TextEncoder().encode(name), (byte) => byte.toString(16).padStart(2, "0")).join("")}`), name, hostId, kind: "bind", source: name, sizeBytes: 0, sizeUnavailable: true, createdAt: "", consumers, labels: {}, inventory });
    }
  }
  return [...rows.filter((r) => r.labels?.["io.docklet.system"] !== "agent").map((r): Volume => {
    if (!r?.name || !r.driver) throw new Error("inventory.invalid-volume");
    const size = r.usageData?.Size ?? (r.size > 0 ? r.size : undefined);
    return { id: inventoryId(hostId, r.name), hostId, name: r.name, kind: "volume", driver: r.driver, source: r.mountpoint, composeName: r.labels?.["com.docker.compose.volume"], ...owner(r.labels, hostId, projects), sizeBytes: typeof size === "number" && size >= 0 ? size : 0, sizeUnavailable: size == null || size < 0, createdAt: r.createdAt || "", labels: r.labels ?? {}, consumers: usage.get(`volume:${r.name}`) ?? [], inventory };
  }), ...binds.values()];
}

export function mapTopology(rows: Schema["NetworkInspect"][], hostId: string, containers: ContainerRow[], projects: ProjectRow[], inventory: InventoryObservation): Topology {
  const networks: TopologyNetwork[] = rows.filter((r) => r.labels?.["io.docklet.system"] !== "agent").map((r) => {
    if (!r?.id || !r.name || typeof r.driver !== "string" || !r.driver) throw new Error("inventory.invalid-network");
    const { internal } = r;
    const ip4 = r.ipam.config?.find((c) => c.subnet && !c.subnet.includes(":"));
    const ip6 = r.ipam.config?.find((c) => c.subnet?.includes(":"));
    return { id: inventoryId(hostId, r.id), hostId, name: r.name, driver: r.driver, internal, builtIn: ["bridge", "host", "none"].includes(r.name), ipv4: r.enableIPv4, ipv6: r.enableIPv6, subnet: ip4?.subnet, gateway: ip4?.gateway, ipRange: ip4?.ipRange, subnetConfigured: !!ip4?.subnet, subnet6: ip6?.subnet, gateway6: ip6?.gateway, ipRange6: ip6?.ipRange, subnet6Configured: !!ip6?.subnet, parent: r.options?.parent, ...owner(r.labels, hostId, projects), inventory };
  });
  const users = containers.filter((c) => c.labels?.["io.docklet.system"] !== "agent");
  const attachments: Topology["attachments"] = [];
  const dependencies: Topology["dependencies"] = [];
  for (const c of users) {
    const containerId = inventoryId(hostId, c.id);
    for (const n of networks) {
      const endpoint = c.networkSettings?.networks?.[n.name];
      const raw = rows.find((r) => inventoryId(hostId, r.id) === n.id)?.containers?.[c.id];
      if (!endpoint && !raw) continue;
      attachments.push({ containerId, networkId: n.id, ip: endpoint?.ipAddress || raw?.IPv4Address?.split("/")[0] || undefined, ip6: endpoint?.globalIPv6Address || raw?.IPv6Address?.split("/")[0] || undefined });
    }
    if (!c.labels?.["com.docker.compose.project"]) continue;
    for (const entry of (c.labels?.["com.docker.compose.depends_on"] ?? "").split(",").filter(Boolean)) {
      const service = entry.split(":")[0];
      for (const target of users.filter((candidate) => candidate.id !== c.id && candidate.labels?.["com.docker.compose.project"] === c.labels?.["com.docker.compose.project"] && candidate.labels?.["com.docker.compose.service"] === service)) dependencies.push({ from: containerId, to: inventoryId(hostId, target.id) });
    }
  }
  return { networks, attachments, dependencies };
}
