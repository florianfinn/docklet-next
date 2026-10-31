import { sameImageReference, imageReferenceValue } from "@/domain/docker-reference";
import { checkResult } from "./update-decoders";
// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original inventory DTO projections.

import type { components } from "@/api/generated/schema";
import type { ContainerInfo, ContainerState, Health, ImageVersion, InventoryObservation, Service, Stack } from "@/domain/model";
import { SYSTEM_LABEL } from "@/domain/system";
import { mapUpdateInfo } from "./update-mapping";
import { composeProjectOwner } from "./project-ownership";

export type ProjectRow = components["schemas"]["ProjectDetails"];
export type ContainerRow = components["schemas"]["ContainerSummary"];

export function inventoryId(hostId: string, id: string): string {
  if (!hostId || !id || hostId.includes("~") || id.includes("~")) throw new Error("inventory.invalid-id");
  return `${hostId}~${id}`;
}

export function splitInventoryId(id: string): [string, string] {
  const parts = id.split("~");
  if (parts.length !== 2 || parts.some((part) => !part)) throw new Error("inventory.invalid-id");
  return parts as [string, string];
}

function imageVersion(ref: string): ImageVersion {
  if (!ref) throw new Error("inventory.invalid-image");
  const name = ref.split("@")[0] ?? "";
  const colon = name.lastIndexOf(":");
  return colon > name.lastIndexOf("/") ? { repository: name.slice(0, colon), tag: name.slice(colon + 1) } : { repository: name, tag: "" };
}

function state(value: string): ContainerState {
  if (value === "running" || value === "restarting") return value;
  if (["exited", "dead", "created", "stopped"].includes(value)) return "stopped";
  return "unknown";
}
function health(value: string): Health {
  if (value.includes("(unhealthy)")) return "unhealthy";
  if (value.includes("(healthy)")) return "healthy";
  if (value.includes("(starting)")) return "starting";
  return "none";
}

export function mapContainer(row: ContainerRow, hostId: string, projects: ProjectRow[], inventory: InventoryObservation): ContainerInfo {
  if (!row || !row.id || !Array.isArray(row.names) || !row.names[0] || typeof row.state !== "string" || typeof row.status !== "string") throw new Error("inventory.invalid-container");
  const labels = row.labels ?? {};
  const project = labels["com.docker.compose.project"];
  const service = labels["com.docker.compose.service"];
  // Older responses without origin stay unknown; labels never grant editing authority.
  const origin = row.origin ?? "unknown";
  const originSource = row.originSource ?? "detected";
  const owner = composeProjectOwner(projects, project, row.id);
  const image = imageVersion(row.image);
  return {
    imageId: row.imageId ? inventoryId(hostId, row.imageId) : undefined,
    id: inventoryId(hostId, row.id), name: row.names[0].replace(/^\//, ""), hostId,
    ...(owner && origin === "compose" ? { stackId: inventoryId(hostId, owner.id), stackName: owner.name } : {}),
    ...(service ? { serviceName: service } : {}), origin, originSource, image,
    state: state(row.state), health: health(row.status),
    ports: (row.ports ?? []).filter((port, index, ports) => ports.findIndex((other) => other.publicPort === port.publicPort && other.privatePort === port.privatePort && other.type === port.type) === index).map((port) => ({ container: String(port.privatePort), ...(port.publicPort ? { host: String(port.publicPort) } : {}), ...(port.type === "tcp" || port.type === "udp" ? { protocol: port.type } : {}) })),
    networks: Object.keys(row.networkSettings?.networks ?? {}), mounts: (row.mounts ?? []).map((mount) => mount.name || mount.destination),
    env: {}, restartPolicy: row.hostConfig?.restartPolicy ?? "", memLimitMb: (row.hostConfig?.memory ?? 0) / 1024 / 1024,
    update: mapUpdateInfo(row.updateInfo, image, row.image, inventory.stale), dockerLabels: labels, inventory,
  };
}

export function mapStack(row: ProjectRow, hostId: string, containers: ContainerInfo[], inventory: InventoryObservation): Stack {
  if (!row?.id || !row.name || typeof row.status !== "string") throw new Error("inventory.invalid-project");
  if (row.updateInfo) checkResult(row.updateInfo);
  const incomplete = !!row.updateInfo && (row.updateInfo.checkedImageCount < row.updateInfo.imageCount || row.updateInfo.errorCount > 0 || row.updateInfo.imagesNotPulled > 0);
  const members = containers.filter((item) => item.stackId === inventoryId(hostId, row.id));
  // Missing or unowned inventory cannot disprove explicit backend membership.
  // Only a concrete conflicting web owner excludes a runtime entry.
  const runtimeServices = row.runtimeServices?.filter((runtime) => {
    const id = runtime.containerId;
    return !id || !containers.some((item) => item.id === inventoryId(hostId, id) && !!item.stackId && item.stackId !== inventoryId(hostId, row.id));
  });
  const services: Service[] = (runtimeServices?.length ? runtimeServices : members.map((c) => ({ name: c.serviceName ?? c.name, image: `${c.image.repository}:${c.image.tag}`, status: c.state, health: c.health, containerId: splitInventoryId(c.id)[1] }))).map((runtime) => {
    const container = members.find((item) => item.id === inventoryId(hostId, runtime.containerId || "absent"));
    const image = imageVersion(runtime.image);
    return { name: runtime.name, image, state: state(runtime.status), health: runtime.health === "healthy" || runtime.health === "unhealthy" || runtime.health === "starting" ? runtime.health : container?.health ?? "none",
      ports: container?.ports ?? [], networks: container?.networks ?? [], restartPolicy: container?.restartPolicy ?? "", memLimitMb: container?.memLimitMb ?? 0, restarts24h: 0,
      update: { ...mapUpdateInfo(row.updateInfo?.serviceUpdates?.[runtime.name] ? (sameImageReference(row.updateInfo.serviceUpdates[runtime.name]!.imageRef, runtime.image) ? row.updateInfo.serviceUpdates[runtime.name]?.updateInfo : undefined) : imageReferenceValue(row.updateInfo?.updateInfoByRef, runtime.image), image, runtime.image, inventory.stale), incomplete } };
  });
  return { id: inventoryId(hostId, row.id), hostId, name: row.name, path: row.path, reportedStatus: row.status, serviceCount: row.serviceCount, runningCount: row.runningCount, deployHistoryUnavailable: true, labels: (row.tags ?? []).map((tag) => tag.name), services, autoUpdate: { mode: "check" }, inventory,
    ...(members.some((item) => item.dockerLabels?.[SYSTEM_LABEL] === "agent") ? { dockerLabels: { [SYSTEM_LABEL]: "agent" } } : {}) };
}
