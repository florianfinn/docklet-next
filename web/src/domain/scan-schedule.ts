// Vulnerability scan schedule: a subset of the shared plan schedule (manual,
// daily or weekly), stored in the hub settings and mirrored on the security
// overview. Wall-clock times in the hub time zone, like every other schedule.

import { DEFAULT_TIME, nextOccurrence, validateSchedule, type PlanSchedule, type ScheduleError } from "./schedule";

export type ScanSchedule = Extract<PlanSchedule, { mode: "manual" | "daily" | "weekly" }>;
export type ScanRecurrence = ScanSchedule["mode"];

export const SCAN_MODES: readonly ScanRecurrence[] = ["manual", "daily", "weekly"];

export const DEFAULT_SCAN_SCHEDULE: ScanSchedule = { mode: "daily", time: DEFAULT_TIME };

/**
 * Reads a stored scan schedule. Earlier data held only the recurrence name
 * ("daily", "weekly"); it becomes a typed schedule at DEFAULT_TIME, weekly on
 * Sunday. Anything unknown falls back to the default.
 */
export function normalizeScanSchedule(value: unknown): ScanSchedule {
  if (value === "manual") return { mode: "manual" };
  if (value === "daily") return { mode: "daily", time: DEFAULT_TIME };
  if (value === "weekly") return { mode: "weekly", day: "sun", time: DEFAULT_TIME };
  if (value && typeof value === "object" && "mode" in value) {
    const s = value as ScanSchedule;
    if (s.mode === "manual" || s.mode === "daily" || s.mode === "weekly") return s;
  }
  return DEFAULT_SCAN_SCHEDULE;
}

/** Problem of a recurring scan schedule; manual needs nothing. */
export function validateScanSchedule(schedule: ScanSchedule, now: Date, timeZone: string): ScheduleError | null {
  return schedule.mode === "manual" ? null : validateSchedule(schedule, now, timeZone);
}

/** Next planned scan; none while scans are off, manual or the schedule is invalid. */
export function nextScan(schedule: ScanSchedule, enabled: boolean, now: Date, timeZone: string): Date | undefined {
  if (!enabled || schedule.mode === "manual" || validateScanSchedule(schedule, now, timeZone)) return undefined;
  return nextOccurrence(schedule, now, timeZone);
}

/** Fragment id of the scan schedule in Settings › Scans; the security page links to it. */
export const SCAN_SCHEDULE_ANCHOR = "scan-schedule";
