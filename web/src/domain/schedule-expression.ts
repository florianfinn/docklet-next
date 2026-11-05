// Source: docklet-next/issues/346
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original projection of scheduler expressions under the shared wall-clock rule.

import { addDays, zonedParts, zonedTimeToInstant, parseDate } from "./schedule";

function field(value: string, min: number, max: number): number[] {
  const selected = new Set<number>();
  for (const item of value.split(",")) {
    const [range, stepText] = item.split("/"), step = stepText === undefined ? 1 : Number(stepText);
    const [first, last] = range === "*" ? [min, max] : range!.split("-").map(Number);
    if (!Number.isInteger(step) || step < 1 || !Number.isInteger(first) || first! < min || (last ?? first)! > max) throw new Error("schedule.invalid-expression");
    for (let n = first!; n <= (last ?? first)!; n += step) selected.add(n);
  }
  return [...selected].sort((a, b) => a - b);
}
/** Read-only projection; all-hours schedules retain elapsed-time fold occurrences. */
export function nextScheduleExpression(expression: string, now: Date, zone: string): Date | undefined {
  if (expression.startsWith("@every ")) {
    const match = /^@every (\d+)(h|m|s)$/.exec(expression);
    if (!match) throw new Error("schedule.invalid-expression");
    return new Date(now.getTime() + Number(match[1]) * ({ h: 3_600_000, m: 60_000, s: 1_000 }[match[2]!]!));
  }
  expression = ({ "@hourly": "0 0 * * * *", "@daily": "0 0 0 * * *", "@weekly": "0 0 0 * * 0", "@monthly": "0 0 0 1 * *", "@yearly": "0 0 0 1 1 *" } as Record<string, string>)[expression] ?? expression;
  const fields = expression.trim().split(/\s+/);
  if (fields.length !== 6 || fields[0] !== "0") throw new Error("schedule.invalid-expression");
  const minutes = field(fields[1]!, 0, 59), hours = field(fields[2]!, 0, 23), days = field(fields[3]!, 1, 31), months = field(fields[4]!, 1, 12), weekdays = field(fields[5]!, 0, 6);
  const dateMatches = (date: string) => {
    const p = parseDate(date)!, weekday = new Date(Date.UTC(p.y, p.m - 1, p.d)).getUTCDay();
    const dom = days.includes(p.d), dow = weekdays.includes(weekday);
    return months.includes(p.m) && (fields[3] === "*" || fields[5] === "*" ? dom && dow : dom || dow);
  };
  const start = zonedParts(new Date(now.getTime() - 3 * 3_600_000), zone).date;
  if (hours.length === 24) {
    // Search actual minutes so the repeated hour remains observable.
    for (let at = Math.floor(now.getTime() / 60_000) * 60_000 + 60_000, end = at + 366 * 5 * 86_400_000; at < end; at += 60_000) {
      const z = zonedParts(new Date(at), zone);
      if (dateMatches(z.date) && minutes.includes(z.mi)) return new Date(at);
    }
    return undefined;
  }
  for (let i = 0; i < 366 * 5; i++) {
    const date = addDays(start, i);
    if (!dateMatches(date)) continue;
    let best: Date | undefined;
    for (const hour of hours) for (const minute of minutes) {
      const at = zonedTimeToInstant(date, `${String(hour).padStart(2, "0")}:${String(minute).padStart(2, "0")}`, zone);
      if (at > now && (!best || at < best)) best = at;
    }
    if (best) return best;
  }
  return undefined;
}
