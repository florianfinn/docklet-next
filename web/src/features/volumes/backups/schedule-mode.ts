// Source: docklet-next/issues/291
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original manual-plan schedule preservation.

import { backupCron, backupSchedule } from "@/data/http/backup-mapping";
import type { BackupPolicy, BackupSchedule } from "@/domain/volumes";

/** Manual is the disabled plan: it keeps the previous cron so a later switch or save restores it. */
export function withMode(schedule: BackupSchedule, mode: "manual" | "daily" | "weekly"): Pick<BackupPolicy, "enabled" | "schedule"> {
  if (mode === "manual") return { enabled: false, schedule: { mode, cron: backupCron(schedule) } };
  const previous = schedule.mode === "manual" && schedule.cron ? backupSchedule(schedule.cron) : schedule;
  const time = previous.mode === "daily" || previous.mode === "weekly" ? previous.time : "03:00";
  return {
    enabled: true,
    schedule: mode === "daily" ? { mode, time } : { mode, time, day: previous.mode === "weekly" ? previous.day : "sun" },
  };
}
