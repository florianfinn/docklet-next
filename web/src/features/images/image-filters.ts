import { imageReference, imageUsage, type DockerImage, type ImageUsage } from "@/domain/images";
import type { Host } from "@/domain/model";
import { hasUpdate } from "@/domain/updates";
import { sortRows, type SortDir, type SortState } from "@/shared/lib/table-sort";

export type UsageFilter = "all" | ImageUsage;
export type ImageSortKey = "name" | "version" | "host" | "usage" | "consumers" | "size" | "created" | "update";
export type ImageSort = SortState<ImageSortKey>;

/** Default order like the other overviews: host A–Z, names A–Z within each host. */
export const DEFAULT_IMAGE_SORT: ImageSort = { key: "host", dir: "asc" };

export const imageSortDefaultDir = (key: ImageSortKey): SortDir => (key === "size" || key === "created" || key === "consumers" || key === "update" ? "desc" : "asc");

const USAGE_RANK: Record<ImageUsage, number> = { "in-use": 0, unused: 1, dangling: 2 };

export function filterImages(images: readonly DockerImage[], f: { query: string; usage: UsageFilter }): DockerImage[] {
  const q = f.query.trim().toLowerCase();
  return images.filter(
    (i) =>
      (f.usage === "all" || imageUsage(i) === f.usage) &&
      (!q || `${imageReference(i)} ${i.version ?? ""} ${i.aliases.join(" ")} ${i.consumers.map((c) => `${c.name} ${c.stackName ?? ""}`).join(" ")}`.toLowerCase().includes(q)),
  );
}

/** Column sort with the shared tie-breakers (host, then reference, then id). */
export function sortImages(images: readonly DockerImage[], hosts: Map<string, Host>, sort: ImageSort): DockerImage[] {
  return sortRows(
    [...images],
    sort,
    (i, key) => {
      switch (key) {
        case "name":
          return imageReference(i);
        case "version":
          return i.version ?? null;
        case "host":
          return hosts.get(i.hostId)?.name ?? i.hostId;
        case "usage":
          return USAGE_RANK[imageUsage(i)];
        case "consumers":
          return i.consumers.length;
        case "size":
          return i.sizeMb;
        case "created":
          return new Date(i.createdAt).getTime();
        case "update":
          return i.update && hasUpdate(i.update) ? 1 : 0;
      }
    },
    (i) => ({ id: i.id, name: imageReference(i), hostName: hosts.get(i.hostId)?.name ?? i.hostId }),
  );
}
