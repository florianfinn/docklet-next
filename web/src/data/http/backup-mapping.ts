// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original volume backup DTO projection.

import type { components } from "@/api/generated/schema";
import type { BackupOverview, BackupPolicy, BackupRun, BackupSchedule } from "@/domain/volumes";
import { parseSimpleBackupSchedule, formatSimpleBackupSchedule, type BackupWeekday } from "@/features/hub-backups/simple-schedule";
import { inventoryId } from "./inventory-mapping";

type Schema = components["schemas"];
const DAYS = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"] as const;

export function backupSchedule(cron: string): BackupSchedule {
  const parsed = parseSimpleBackupSchedule(cron);
  if (!parsed) return { mode: "custom", cron };
  return parsed.frequency === "daily" ? { mode: "daily", time: parsed.time } : { mode: "weekly", time: parsed.time, day: DAYS[Number(parsed.weekday)]! };
}

export function backupCron(schedule: BackupSchedule): string {
  if (schedule.mode === "custom") return schedule.cron;
  if (schedule.mode === "manual") return "0 0 3 * * *";
  return formatSimpleBackupSchedule(schedule.mode === "daily" ? { frequency: "daily", time: schedule.time } : { frequency: "weekly", time: schedule.time, weekday: String(DAYS.indexOf(schedule.day)) as BackupWeekday });
}

export function mapBackupPolicy(row: Schema["VolumeBackupPolicy"]): BackupPolicy {
  return { id: row.id, enabled: row.enabled, targetId: row.targetId, schedule: backupSchedule(row.schedule), retentionCount: row.retentionCount, stopContainers: row.stopContainers, nextRunAt: row.nextRunAt };
}

export function mapBackup(row: Schema["VolumeBackupEntry"], host: string): BackupRun {
  return { id: row.id, volumeId: inventoryId(host, row.volumeName), targetId: row.destination, startedAt: row.createdAt, status: row.status, trigger: row.trigger, sizeBytes: row.localSnapshotId && Number.isFinite(row.size) && row.size >= 0 ? row.size : undefined, archive: row.localSnapshotId, errorCode: row.errorCode, actionId: row.activityId, stoppedContainers: row.stopContainers };
}

export function mapBackupOverview(dto: Schema["VolumeBackupOverview"], host: string): BackupOverview {
  if (typeof dto.timeZone !== "string" || typeof dto.backupTargetAuthority !== "string" || !Number.isInteger(dto.backupTargetSetVersion) || !dto.repository) throw new Error("backup.invalid-response");
  return { hostId: host, timeZone: dto.timeZone, authority: dto.backupTargetAuthority, setVersion: dto.backupTargetSetVersion, repository: dto.repository, targets: dto.targets ?? [], volumes: (dto.volumes ?? []).map((v) => ({
    id: inventoryId(host, v.name), name: v.name, hostId: host, kind: "volume", source: "", createdAt: "", sizeBytes: 0, sizeUnavailable: true, consumers: [], labels: {}, volumeExists: v.volumeExists,
    backup: v.policy ? mapBackupPolicy(v.policy) : undefined, lastBackup: v.lastBackup ? mapBackup(v.lastBackup, host) : undefined, running: v.running, lastRestore: v.lastRestore, pendingRestores: v.pendingRestores ?? [], timeZone: dto.timeZone,
  })) };
}
