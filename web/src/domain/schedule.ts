// Typed schedules for backups and scheduled update installs. Times are wall
// clock times in the hub time zone (Settings › General), never in the
// browser's zone, so every viewer sees the same next run.

export type Weekday = "mon" | "tue" | "wed" | "thu" | "fri" | "sat" | "sun";
export const WEEKDAYS: readonly Weekday[] = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"];
const WEEKDAY_INDEX: Record<Weekday, number> = { sun: 0, mon: 1, tue: 2, wed: 3, thu: 4, fri: 5, sat: 6 };

/**
 * One run at a date, or a recurrence. `date` and `from` are YYYY-MM-DD,
 * `time` HH:MM (24 h). A weekly schedule may start at a chosen first date.
 */
export type Schedule = { mode: "once"; date: string; time: string } | { mode: "daily"; time: string } | { mode: "weekly"; day: Weekday; time: string; from?: string };
export type Recurrence = Schedule["mode"];

export const DEFAULT_TIME = "03:00";
/** Starting point when scheduled update installs are switched on. */
export const DEFAULT_UPDATE_SCHEDULE: Schedule = { mode: "weekly", day: "sun", time: "04:00" };
export const DEFAULT_TIME_ZONE = "Europe/Berlin";

export type ScheduleError = "time" | "date" | "past" | "day";

/** Backup plans may also run only on demand. */
export type PlanSchedule = { mode: "manual" } | Schedule;

const TIME = /^([01]\d|2[0-3]):([0-5]\d)$/;
const DATE = /^(\d{4})-(\d{2})-(\d{2})$/;

export function isTime(value: string): boolean {
  return TIME.test(value);
}

/** Calendar date parts of YYYY-MM-DD; undefined for impossible dates such as 2026-02-30. */
export function parseDate(value: string): { y: number; m: number; d: number } | undefined {
  const match = DATE.exec(value);
  if (!match) return undefined;
  const [y, m, d] = [Number(match[1]), Number(match[2]), Number(match[3])];
  const check = new Date(Date.UTC(y, m - 1, d));
  return check.getUTCFullYear() === y && check.getUTCMonth() === m - 1 && check.getUTCDate() === d ? { y, m, d } : undefined;
}

export function toIsoDate(y: number, m: number, d: number): string {
  return `${String(y).padStart(4, "0")}-${String(m).padStart(2, "0")}-${String(d).padStart(2, "0")}`;
}

/** Weekday of a calendar date (independent of any time zone). */
export function weekdayOf(date: string): Weekday | undefined {
  const p = parseDate(date);
  if (!p) return undefined;
  const index = new Date(Date.UTC(p.y, p.m - 1, p.d)).getUTCDay();
  return (Object.keys(WEEKDAY_INDEX) as Weekday[]).find((k) => WEEKDAY_INDEX[k] === index);
}

/** Adds days to a calendar date. */
export function addDays(date: string, days: number): string {
  const p = parseDate(date)!;
  const d = new Date(Date.UTC(p.y, p.m - 1, p.d + days));
  return toIsoDate(d.getUTCFullYear(), d.getUTCMonth() + 1, d.getUTCDate());
}

const partFormatters = new Map<string, Intl.DateTimeFormat>();

function formatterFor(timeZone: string): Intl.DateTimeFormat {
  let f = partFormatters.get(timeZone);
  if (!f) {
    f = new Intl.DateTimeFormat("en-CA", { timeZone, hourCycle: "h23", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit" });
    partFormatters.set(timeZone, f);
  }
  return f;
}

/** Wall clock date and time of an instant in a time zone. */
export function zonedParts(instant: Date, timeZone: string): { date: string; time: string; y: number; m: number; d: number; h: number; mi: number; s: number } {
  const parts = Object.fromEntries(formatterFor(timeZone).formatToParts(instant).map((p) => [p.type, p.value]));
  const [y, m, d, h, mi, s] = [Number(parts.year), Number(parts.month), Number(parts.day), Number(parts.hour) % 24, Number(parts.minute), Number(parts.second)];
  return { date: toIsoDate(y, m, d), time: `${String(h).padStart(2, "0")}:${String(mi).padStart(2, "0")}`, y, m, d, h, mi, s };
}

/**
 * Instant of a wall clock time in a time zone. Two correction passes cover
 * the zone offset and a DST change between guess and result. A time inside a
 * spring-forward gap moves on by the skipped length (02:30 runs at 03:30).
 */
export function zonedTimeToInstant(date: string, time: string, timeZone: string): Date {
  const p = parseDate(date)!;
  const [h, mi] = time.split(":").map(Number) as [number, number];
  const target = Date.UTC(p.y, p.m - 1, p.d, h, mi);
  let guess = target;
  for (let i = 0; i < 2; i++) {
    const z = zonedParts(new Date(guess), timeZone);
    const wall = Date.UTC(z.y, z.m - 1, z.d, z.h, z.mi, z.s);
    guess += target - wall;
  }
  return new Date(guess);
}

/** Next run strictly after `now`; undefined for a one-time schedule in the past or invalid input. */
export function nextOccurrence(schedule: Schedule, now: Date, timeZone: string = DEFAULT_TIME_ZONE): Date | undefined {
  if (!isTime(schedule.time)) return undefined;
  if (schedule.mode === "once") {
    if (!parseDate(schedule.date)) return undefined;
    const at = zonedTimeToInstant(schedule.date, schedule.time, timeZone);
    return at > now ? at : undefined;
  }
  const today = zonedParts(now, timeZone).date;
  const start = schedule.mode === "weekly" && schedule.from && parseDate(schedule.from) && schedule.from > today ? schedule.from : today;
  for (let i = 0; i <= 8; i++) {
    const date = addDays(start, i);
    if (schedule.mode === "weekly" && weekdayOf(date) !== schedule.day) continue;
    const at = zonedTimeToInstant(date, schedule.time, timeZone);
    if (at > now) return at;
  }
  return undefined;
}

export function validateSchedule(schedule: Schedule, now: Date, timeZone: string = DEFAULT_TIME_ZONE): ScheduleError | null {
  if (!isTime(schedule.time)) return "time";
  if (schedule.mode === "weekly" && !WEEKDAYS.includes(schedule.day)) return "day";
  if (schedule.mode === "weekly" && schedule.from !== undefined && !parseDate(schedule.from)) return "date";
  if (schedule.mode === "once") {
    if (!parseDate(schedule.date)) return "date";
    if (!nextOccurrence(schedule, now, timeZone)) return "past";
  }
  return null;
}

/**
 * Switches the recurrence and keeps what still applies: the time always, the
 * weekday of a one-time date. A new one-time schedule starts without a date,
 * so the date is always an explicit choice. Manual plans start at DEFAULT_TIME.
 */
export function withRecurrence(schedule: PlanSchedule, mode: PlanSchedule["mode"], now: Date, timeZone: string = DEFAULT_TIME_ZONE): PlanSchedule {
  if (schedule.mode === mode) return schedule;
  if (mode === "manual") return { mode };
  const time = schedule.mode !== "manual" && isTime(schedule.time) ? schedule.time : DEFAULT_TIME;
  if (mode === "daily") return { mode, time };
  if (mode === "weekly") {
    const day = schedule.mode === "once" ? (weekdayOf(schedule.date) ?? "sun") : weekdayOf(zonedParts(now, timeZone).date)!;
    return { mode, day, time };
  }
  return { mode, date: "", time };
}

/** Former free-text windows such as "Mi 03:30" or "So 04:00", and plain "03:30". */
export function parseLegacyWindow(value: string | undefined): Schedule | undefined {
  const m = /^\s*(?:(mo|di|mi|do|fr|sa|so)\w*\.?\s+)?([01]?\d|2[0-3]):([0-5]\d)\s*$/i.exec(value ?? "");
  if (!m) return undefined;
  const time = `${m[2]!.padStart(2, "0")}:${m[3]}`;
  const days: Record<string, Weekday> = { mo: "mon", di: "tue", mi: "wed", do: "thu", fr: "fri", sa: "sat", so: "sun" };
  return m[1] ? { mode: "weekly", day: days[m[1].toLowerCase()]!, time } : { mode: "daily", time };
}
