import { useTranslations } from "use-intl";

import { intlTag } from "@/domain/locale";
import { nextOccurrence, parseDate, type PlanSchedule } from "@/domain/schedule";

/** "in 40 Minuten", "in 5 Stunden", "in 3 Tagen": minutes below one hour, hours below two days. */
export function relativeFuture(at: Date, now: Date): string {
  const relative = new Intl.RelativeTimeFormat(intlTag(), { numeric: "always" });
  const minutes = Math.max(1, Math.round((at.getTime() - now.getTime()) / 60_000));
  if (minutes < 60) return relative.format(minutes, "minute");
  const hours = Math.round(minutes / 60);
  if (hours < 48) return relative.format(hours, "hour");
  return relative.format(Math.round(hours / 24), "day");
}

/** Short zone name at an instant, e.g. "MESZ" for Europe/Berlin in summer. */
export function zoneAbbreviation(at: Date, timeZone: string): string {
  return new Intl.DateTimeFormat(intlTag(), { timeZone, timeZoneName: "short" }).formatToParts(at).find((p) => p.type === "timeZoneName")?.value ?? timeZone;
}

/** Summary, next run and relative time of a schedule in the interface language, always in the hub time zone. */
export function useScheduleText() {
  const t = useTranslations("common.schedule");
  const dateFmt = (date: string) => {
    const p = parseDate(date);
    return p ? new Intl.DateTimeFormat(intlTag(), { day: "2-digit", month: "2-digit", year: "numeric", timeZone: "UTC" }).format(new Date(Date.UTC(p.y, p.m - 1, p.d))) : date;
  };
  const summary = (s: PlanSchedule): string => {
    if (s.mode === "manual") return t("manual");
    if (s.mode === "once") return t("summary.once", { date: dateFmt(s.date), time: s.time });
    if (s.mode === "daily") return t("summary.daily", { time: s.time });
    return s.from ? t("summary.weeklyFrom", { day: t(`daysPlural.${s.day}`), time: s.time, from: dateFmt(s.from) }) : t("summary.weekly", { day: t(`daysPlural.${s.day}`), time: s.time });
  };
  const next = (s: PlanSchedule, now: Date, timeZone: string) => {
    const at = s.mode === "manual" ? undefined : nextOccurrence(s, now, timeZone);
    if (!at) return undefined;
    const when = new Intl.DateTimeFormat(intlTag(), { timeZone, weekday: "short", day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" }).format(at);
    return { at, when, relative: relativeFuture(at, now), zone: `${timeZone} (${zoneAbbreviation(at, timeZone)})` };
  };
  return { summary, next };
}
