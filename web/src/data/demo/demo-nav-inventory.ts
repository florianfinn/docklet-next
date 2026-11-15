// Source: docklet-next/issues/462
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original demo shell projection from raw Compose configuration and explicit standalone sources.

import { parse } from "yaml";
import { imageRef } from "@/domain/updates";
import { mountRef } from "@/domain/volumes";
import { withoutSystem } from "@/domain/system";
import type { ContainerInfo } from "@/domain/model";
import type { DemoDb } from "./demo-db";
import { distinctBindCount, type InventoryContainer, type NavInventory } from "../nav-inventory";

/** Read the source configuration, before its short mount syntax is projected for display. */
export function demoMountSources(compose: string, service: string, container: Pick<ContainerInfo, "hostId" | "stackId">, projectDir?: string) {
  let document: { services?: Record<string, { volumes?: (string | { type?: string; source?: string })[] }> } | null;
  try { document = parse(compose); } catch { return { binds: [], volumes: [] }; }
  const rawDocument = document as { services?: Record<string, { volumes?: (string | { type?: string; source?: string })[] }> } | null;
  const binds: string[] = [], volumes: string[] = [];
  for (const mount of rawDocument?.services?.[service]?.volumes ?? []) {
    if (typeof mount === "string") {
      const ref = mountRef(container, mount, projectDir);
      if (ref) (ref.kind === "bind" ? binds : volumes).push(ref.name);
    } else if (mount.source) {
      if (mount.type === "bind") binds.push(mount.source);
      else if (mount.type === "volume") volumes.push(container.stackId ? `${container.stackId}_${mount.source}` : mount.source);
    }
  }
  return { binds, volumes };
}

export function demoInventoryContainers(db: DemoDb): InventoryContainer[] {
  return db.discoveredContainers().filter((c) => !!db.host(c.hostId)).map((c) => {
    const raw = c.stackId && c.serviceName ? demoMountSources(db.compose[c.stackId] ?? "", c.serviceName, c, db.host(c.hostId)?.projectDir) : undefined;
    const standalone = db.standalone.find((row) => row.id === c.id) as (ContainerInfo & { bindSources?: string[] }) | undefined;
    const image = db.security.images.find((i) => i.hostId === c.hostId && i.ref === imageRef(c.image));
    return { ...c, imageId: image ? `${c.hostId}~${image.id ?? image.ref}` : undefined, bindSources: raw?.binds ?? standalone?.bindSources ?? [] };
  });
}

export function demoNavInventory(db: DemoDb): NavInventory[] {
  const raw = demoInventoryContainers(db);
  return db.hosts.map((host) => {
    const names = new Set([...db.createdVolumes.values()].filter((v) => v.hostId === host.id).map((v) => v.name));
    const containers = raw.filter((c) => c.hostId === host.id);
    for (const c of withoutSystem(containers)) {
      const configuration = c.stackId && c.serviceName ? demoMountSources(db.compose[c.stackId] ?? "", c.serviceName, c, host.projectDir) : undefined;
      const standalone = db.standalone.find((row) => row.id === c.id) as (ContainerInfo & { volumeNames?: string[] }) | undefined;
      for (const name of configuration?.volumes ?? standalone?.volumeNames ?? []) names.add(name);
    }
    return { hostId: host.id, containers, inventoryUnavailable: false, volumeTotal: names.size + distinctBindCount(containers) };
  });
}
