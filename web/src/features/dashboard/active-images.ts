// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original Dashboard security view from active counts and the container inventory (AUTO-6).

import type { ActiveSecurity, ContainerInfo, ScannedImage, SecurityOverview } from "@/domain/model";

const shortId = (id: string) => id.replace(/^sha256:/, "").slice(0, 12);
const inventoryImageId = (hostId: string, id: string) => `${hostId}~${id}`;

/**
 * The Dashboard's security overview: active counts per scanned image, named and attributed by the containers that
 * use it (an unused image keeps its own name if the source knows it, else its short id). New objects throughout.
 */
export function activeOverview(active: ActiveSecurity, containers: readonly ContainerInfo[]): SecurityOverview {
  const users = new Map<string, ContainerInfo[]>();
  for (const c of containers) {
    if (!c.imageId) continue;
    const key = `${c.hostId}\u0000${c.imageId}`;
    const list = users.get(key);
    if (list) list.push(c); else users.set(key, [c]);
  }
  const images: ScannedImage[] = active.images.map((image) => {
    // Containers carry the inventory image id (`host~sha256:…`); the active counts carry the raw image id.
    const used = users.get(`${image.hostId}\u0000${inventoryImageId(image.hostId, image.id)}`) ?? [];
    const first = used[0];
    const ref = first ? `${first.image.repository}:${first.image.tag}` : image.ref ?? shortId(image.id);
    const usage = used.some((c) => c.state === "running") ? "running" : used.length || image.inUse ? "stopped" : "unused";
    return { id: `${image.hostId}:${image.id}`, ref, hostId: image.hostId, stackId: first?.stackId, usage, counts: { ...image.counts }, scannedAt: image.scannedAt, ...(image.unavailable ? { unavailable: true } : {}) };
  });
  return { enabled: active.enabled, schedule: { mode: "manual" }, images, findings: [], history: [], unavailable: active.unavailable.map((u) => ({ ...u })) };
}
