import type { Host } from "@/domain/model";
import type { BackupTarget } from "@/domain/settings";
import { volumeLabel, type Volume } from "@/domain/volumes";
import { sortRows, type SortDir, type SortState } from "@/shared/lib/table-sort";

export type BackupSortKey = "name" | "host" | "target" | "schedule" | "next" | "last";
export type BackupSort = SortState<BackupSortKey>;

/** Default like before: the next run first. */
export const DEFAULT_BACKUP_SORT: BackupSort = { key: "next", dir: "asc" };
export const backupSortDefaultDir = (_key: BackupSortKey): SortDir => "asc";

const SCHEDULE_RANK = { custom: 0, daily: 1, weekly: 2, manual: 3 } as const;

/** Failed runs first, then running, never run and successful ones. */
function lastRank(v: Volume): number {
  const s = v.lastBackup?.status;
  return s === "failed" ? 0 : s === "running" ? 1 : !s ? 2 : 3;
}

/**
 * Header sort of the backups overview with the shared tie-breakers (host,
 * name, id). Plans without a next run (manual or paused) come last.
 */
export function sortBackups(volumes: Volume[], hosts: Map<string, Host>, targets: BackupTarget[], sort: BackupSort, _now?: Date, _timeZone?: string): Volume[] {
  return sortRows(
    volumes,
    sort,
    (v, key) => {
      const plan = v.backup;
      switch (key) {
        case "name":
          return volumeLabel(v);
        case "host":
          return hosts.get(v.hostId)?.name ?? v.hostId;
        case "target":
          return targets.find((t) => t.id === plan?.targetId)?.name ?? "";
        case "schedule":
          return !plan ? null : SCHEDULE_RANK[plan.schedule.mode] * 10_000 + (plan.schedule.mode === "manual" || plan.schedule.mode === "custom" ? 0 : Number(plan.schedule.time.replace(":", "")));
        case "next":
          return plan?.enabled && plan.nextRunAt ? Date.parse(plan.nextRunAt) : null;
        case "last":
          return lastRank(v);
      }
    },
    (v) => ({ id: v.id, name: volumeLabel(v), hostName: hosts.get(v.hostId)?.name ?? v.hostId }),
  );
}
