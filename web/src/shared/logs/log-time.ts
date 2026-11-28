// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original local log time formatting for the M47 log views.

import { activeLocale, INTL_LOCALE } from "@/domain/locale";

const stamps = new Map<string, Intl.DateTimeFormat>();
/** Local date and time of a log instant, to the second (K-8). */
export function formatLogStamp(ms: number, withDate = true): string {
  if (!Number.isFinite(ms)) return "";
  const locale = INTL_LOCALE[activeLocale()], key = `${locale}|${withDate}`;
  let format = stamps.get(key);
  if (!format) stamps.set(key, (format = new Intl.DateTimeFormat(locale, {
    ...(withDate ? { day: "2-digit", month: "2-digit" } : {}),
    hour: "2-digit", minute: "2-digit", second: "2-digit",
  })));
  return format.format(new Date(ms));
}
/** "start – end" with the date written once when both ends share a day, or one instant when both share a second. */
export function formatLogSpan(first: number, last: number): string {
  const sameDay = new Date(first).toDateString() === new Date(last).toDateString();
  const start = formatLogStamp(first), end = formatLogStamp(last, !sameDay);
  return start === formatLogStamp(last) ? start : `${start} – ${end}`;
}
