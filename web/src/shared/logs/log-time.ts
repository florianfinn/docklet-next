// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original local log time formatting for the M47 log views.

import { activeLocale, INTL_LOCALE } from "@/domain/locale";

const stamps = new Map<string, Intl.DateTimeFormat>();
/** Local date and time of a log instant; milliseconds when the surrounding span is shorter than a minute. */
export function formatLogStamp(ms: number, millis: boolean, withDate = true): string {
  if (!Number.isFinite(ms)) return "";
  const locale = INTL_LOCALE[activeLocale()], key = `${locale}|${millis}|${withDate}`;
  let format = stamps.get(key);
  if (!format) stamps.set(key, (format = new Intl.DateTimeFormat(locale, {
    ...(withDate ? { day: "2-digit", month: "2-digit" } : {}),
    hour: "2-digit", minute: "2-digit", second: "2-digit", ...(millis ? { fractionalSecondDigits: 3 } : {}),
  } as Intl.DateTimeFormatOptions)));
  return format.format(new Date(ms));
}
/** "start – end" with the date written once when both ends share a day. */
export function formatLogSpan(first: number, last: number): string {
  const millis = last - first < 60_000;
  const sameDay = new Date(first).toDateString() === new Date(last).toDateString();
  return `${formatLogStamp(first, millis)} – ${formatLogStamp(last, millis, !sameDay)}`;
}
