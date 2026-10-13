import type { BackupPolicy, BackupRun } from "@/domain/volumes";

import { daysAgo, hoursAgo } from "./time";

/** Backup policies keyed by volume id (see domain/volumes volumeId). */
export const BACKUP_POLICIES: Record<string, BackupPolicy> = {
  "vol-atlas-paperless_pg_data": { enabled: true, targetId: "t-smb-nas", schedule: { mode: "daily", time: "03:00" }, retention: { maxCount: 7, maxSizeGb: null }, stopContainers: true },
  "vol-atlas-paperless_paperless_data": { enabled: true, targetId: "t-smb-nas", schedule: { mode: "daily", time: "03:15" }, retention: { maxCount: 7, maxSizeGb: 20 }, stopContainers: false },
  "vol-atlas-home_ha_config": { enabled: true, targetId: "t-local-atlas", schedule: { mode: "daily", time: "02:30" }, retention: { maxCount: 14, maxSizeGb: null }, stopContainers: false },
  "vol-boreal-cloud_cloud_data": { enabled: true, targetId: "t-mount-boreal", schedule: { mode: "weekly", day: "sun", time: "04:00" }, retention: { maxCount: null, maxSizeGb: 200 }, stopContainers: true },
  "vol-boreal-cloud_cloud_pg": { enabled: true, targetId: "t-mount-boreal", schedule: { mode: "weekly", day: "sun", time: "04:10" }, retention: { maxCount: 4, maxSizeGb: null }, stopContainers: true },
  "vol-boreal-craft_craft_world": { enabled: true, targetId: "t-nfs-archive", schedule: { mode: "daily", time: "05:00" }, retention: { maxCount: 5, maxSizeGb: 40 }, stopContainers: true },
  "vol-delta-git_forgejo_data": { enabled: true, targetId: "t-smb-nas", schedule: { mode: "weekly", day: "sat", time: "23:30" }, retention: { maxCount: 6, maxSizeGb: null }, stopContainers: false },
};

const run = (id: string, volumeId: string, targetId: string, at: string, sizeMb: number, extra: Partial<BackupRun> = {}): BackupRun => ({
  id,
  volumeId,
  targetId,
  startedAt: at,
  finishedAt: at,
  status: "succeeded",
  trigger: "scheduled",
  sizeBytes: Math.round(sizeMb * 1024 * 1024),
  archive: `${volumeId.replace(/^vol-[^-]+-/, "")}-${at.slice(0, 10)}.tar.zst`,
  stoppedContainers: false,
  ...extra,
});

export const BACKUP_RUNS: BackupRun[] = [
  run("b-1", "vol-atlas-paperless_pg_data", "t-smb-nas", hoursAgo(5), 118, { stoppedContainers: true }),
  run("b-2", "vol-atlas-paperless_pg_data", "t-smb-nas", daysAgo(1.2), 117, { stoppedContainers: true }),
  run("b-3", "vol-atlas-paperless_pg_data", "t-smb-nas", daysAgo(2.2), 115, { stoppedContainers: true }),
  run("b-4", "vol-atlas-paperless_paperless_data", "t-smb-nas", hoursAgo(4.8), 3),
  run("b-5", "vol-atlas-home_ha_config", "t-local-atlas", hoursAgo(5.5), 64),
  run("b-6", "vol-atlas-home_ha_config", "t-local-atlas", daysAgo(1.25), 63),
  run("b-7", "vol-boreal-cloud_cloud_data", "t-mount-boreal", daysAgo(4), 7.9, { stoppedContainers: true }),
  run("b-8", "vol-boreal-cloud_cloud_pg", "t-mount-boreal", daysAgo(4), 210, { stoppedContainers: true }),
  run("b-9", "vol-boreal-craft_craft_world", "t-nfs-archive", hoursAgo(3), 0, {
    status: "failed",
    sizeBytes: undefined,
    archive: undefined,
    error: "NFS-Ziel nicht erreichbar: Zeitüberschreitung beim Mounten",
    stoppedContainers: true,
  }),
  run("b-10", "vol-boreal-craft_craft_world", "t-nfs-archive", daysAgo(1.1), 14, { stoppedContainers: true }),
  run("b-11", "vol-delta-git_forgejo_data", "t-smb-nas", daysAgo(2.4), 5.2),
  run("b-12", "vol-atlas-media_media_config", "t-local-atlas", daysAgo(12), 190, { trigger: "manual" }),
];
