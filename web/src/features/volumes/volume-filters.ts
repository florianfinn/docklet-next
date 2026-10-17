import type { Host } from "@/domain/model";
import { volumeLabel, type StorageKind, type Volume } from "@/domain/volumes";
import { sortRows, type SortDir, type SortState } from "@/shared/lib/table-sort";

export type KindFilter = "all" | StorageKind;
export type UsageFilter = "all" | "used" | "unused";
export type VolumeSortKey = "name" | "kind" | "host" | "consumers" | "size" | "backup";
export type VolumeSort = SortState<VolumeSortKey>;

/** Default order like stacks and containers: host A–Z, names A–Z within each host. */
export const DEFAULT_VOLUME_SORT: VolumeSort = { key: "host", dir: "asc" };

export const volumeSortDefaultDir = (key: VolumeSortKey): SortDir => (key === "size" || key === "consumers" ? "desc" : "asc");

/** Problems first: failed, then running, disabled and missing plans before healthy ones. */
function backupRank(v: Volume): number {
  if (!v.backup) return 3;
  if (!v.backup.enabled) return 2;
  if (v.lastBackup?.status === "failed") return 0;
  if (v.lastBackup?.status === "running") return 1;
  return 4;
}

export function filterVolumes(volumes: Volume[], f: { query: string; kind: KindFilter; usage: UsageFilter }): Volume[] {
  const q = f.query.trim().toLowerCase();
  return volumes
    .filter((v) => (f.kind === "all" || v.kind === f.kind) && (f.usage === "all" || (f.usage === "used" ? v.consumers.length > 0 : v.consumers.length === 0)))
    .filter((v) => !q || `${v.name} ${v.composeName ?? ""} ${v.stackName ?? ""} ${v.consumers.map((c) => c.containerName).join(" ")}`.toLowerCase().includes(q));
}

/** Column sort with the shared tie-breakers (host, then display name, then id). */
export function sortVolumes(volumes: Volume[], hosts: Map<string, Host>, sort: VolumeSort): Volume[] {
  return sortRows(
    volumes,
    sort,
    (v, key) => {
      switch (key) {
        case "name":
          return volumeLabel(v);
        case "kind":
          return v.kind === "volume" ? 0 : 1;
        case "host":
          return hosts.get(v.hostId)?.name ?? v.hostId;
        case "consumers":
          return v.consumers.length;
        case "size":
          return v.sizeBytes;
        case "backup":
          return backupRank(v);
      }
    },
    (v) => ({ id: v.id, name: volumeLabel(v), hostName: hosts.get(v.hostId)?.name ?? v.hostId }),
  );
}
