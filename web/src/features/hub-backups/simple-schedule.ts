// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original simple schedule conversion for issue #32.

export type BackupFrequency = "off" | "daily" | "weekly";
export type BackupWeekday = "0" | "1" | "2" | "3" | "4" | "5" | "6";
export type SimpleBackupSchedule =
  | { frequency: "daily"; time: string }
  | { frequency: "weekly"; time: string; weekday: BackupWeekday };

const weekdayNames: Record<string, BackupWeekday> = {
  SUN: "0", MON: "1", TUE: "2", WED: "3", THU: "4", FRI: "5", SAT: "6",
};

export function validBackupTime(time: string): boolean {
  if (!/^\d{2}:\d{2}$/.test(time)) return false;
  const [hour, minute] = time.split(":").map(Number);
  return hour! <= 23 && minute! <= 59;
}

export function parseSimpleBackupSchedule(cron: string): SimpleBackupSchedule | null {
  const fields = cron.trim().split(/\s+/);
  if (fields.length !== 6) return null;
  const [second, minute, hour, dayOfMonth, month, dayOfWeek] = fields;
  if (!/^0{1,2}$/.test(second!) || dayOfMonth !== "*" || month !== "*" || !/^\d{1,2}$/.test(minute!) || !/^\d{1,2}$/.test(hour!)) return null;
  const time = `${hour!.padStart(2, "0")}:${minute!.padStart(2, "0")}`;
  if (!validBackupTime(time)) return null;
  if (dayOfWeek === "*") return { frequency: "daily", time };
  const numericDay = /^\d{1,2}$/.test(dayOfWeek!) ? Number(dayOfWeek) : NaN;
  const weekday = Number.isInteger(numericDay) ? String(numericDay === 7 ? 0 : numericDay) : weekdayNames[dayOfWeek!.toUpperCase()];
  if (!weekday) return null;
  if (!/^[0-6]$/.test(weekday)) return null;
  return { frequency: "weekly", time, weekday: weekday as BackupWeekday };
}

export function formatSimpleBackupSchedule(schedule: SimpleBackupSchedule): string {
  if (!validBackupTime(schedule.time)) throw new RangeError();
  const [hour, minute] = schedule.time.split(":").map(Number);
  return `0 ${minute} ${hour} * * ${schedule.frequency === "weekly" ? schedule.weekday : "*"}`;
}
