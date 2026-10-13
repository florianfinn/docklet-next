import { ChevronLeft, ChevronRight } from "lucide-react";
import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import { useTranslations } from "use-intl";

import { intlTag } from "@/domain/locale";
import { addDays, parseDate, toIsoDate } from "@/domain/schedule";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";

interface CalendarProps {
  /** Selected date (YYYY-MM-DD). */
  value?: string;
  onChange: (date: string) => void;
  /** Today in the hub time zone; marked and used as the starting month. */
  today: string;
  /** Earliest selectable date. */
  min?: string;
  label: string;
}

const utcFormat = (options: Intl.DateTimeFormatOptions) => () => new Intl.DateTimeFormat(intlTag(), { ...options, timeZone: "UTC" });
const monthTitle = utcFormat({ month: "long", year: "numeric" });
const dayLabel = utcFormat({ weekday: "long", day: "numeric", month: "long", year: "numeric" });
const weekdayShort = utcFormat({ weekday: "short" });
const weekdayLong = utcFormat({ weekday: "long" });
const utc = (date: string) => {
  const p = parseDate(date)!;
  return new Date(Date.UTC(p.y, p.m - 1, p.d));
};

/** Six Monday-first weeks covering the month of `date`. */
function monthGrid(year: number, month: number): string[][] {
  const first = toIsoDate(year, month, 1);
  const offset = (utc(first).getUTCDay() + 6) % 7;
  const start = addDays(first, -offset);
  return Array.from({ length: 6 }, (_, w) => Array.from({ length: 7 }, (_, d) => addDays(start, w * 7 + d)));
}

function shiftMonth(date: string, months: number): string {
  const p = parseDate(date)!;
  const target = new Date(Date.UTC(p.y, p.m - 1 + months, 1));
  const last = new Date(Date.UTC(target.getUTCFullYear(), target.getUTCMonth() + 1, 0)).getUTCDate();
  return toIsoDate(target.getUTCFullYear(), target.getUTCMonth() + 1, Math.min(p.d, last));
}

/**
 * Month grid following the WAI-ARIA date picker pattern: one tab stop on the
 * focused day, arrows move by day and week, Home/End to the week's start and
 * end, Page Up/Down by month (with Shift by year), Enter or Space selects.
 * Dates before `min` are disabled; month and weekday names follow the interface language.
 */
export function Calendar({ value, onChange, today, min, label }: CalendarProps) {
  const t = useTranslations("common.schedule.calendar");
  const captionId = useId();
  const [focus, setFocus] = useState(() => (value && parseDate(value) ? value : today));
  const [movedByKey, setMovedByKey] = useState(false);
  const grid = useRef<HTMLTableElement>(null);
  const f = parseDate(focus)!;
  const weeks = monthGrid(f.y, f.m);

  useEffect(() => {
    if (!movedByKey) return;
    grid.current?.querySelector<HTMLButtonElement>(`[data-date="${focus}"]`)?.focus();
  }, [focus, movedByKey]);

  const move = (next: string) => {
    setMovedByKey(true);
    setFocus(min && next < min ? min : next);
  };
  const onKey = (e: KeyboardEvent<HTMLButtonElement>, date: string) => {
    const dow = (utc(date).getUTCDay() + 6) % 7;
    const steps: Record<string, () => string> = {
      ArrowLeft: () => addDays(date, -1),
      ArrowRight: () => addDays(date, 1),
      ArrowUp: () => addDays(date, -7),
      ArrowDown: () => addDays(date, 7),
      Home: () => addDays(date, -dow),
      End: () => addDays(date, 6 - dow),
      PageUp: () => shiftMonth(date, e.shiftKey ? -12 : -1),
      PageDown: () => shiftMonth(date, e.shiftKey ? 12 : 1),
    };
    const step = steps[e.key];
    if (!step) return;
    e.preventDefault();
    move(step());
  };

  return (
    <div role="group" aria-label={label} data-slot="calendar" className="flex w-[17.5rem] flex-col gap-2">
      <div className="flex items-center justify-between gap-2">
        <Button type="button" variant="ghost" size="icon" className="size-8" aria-label={t("prev")} onClick={() => setFocus(shiftMonth(focus, -1))}>
          <ChevronLeft />
        </Button>
        <span id={captionId} aria-live="polite" className="text-sm font-semibold capitalize">
          {monthTitle().format(utc(focus))}
        </span>
        <Button type="button" variant="ghost" size="icon" className="size-8" aria-label={t("next")} onClick={() => setFocus(shiftMonth(focus, 1))}>
          <ChevronRight />
        </Button>
      </div>
      <table ref={grid} role="grid" aria-labelledby={captionId} className="w-full border-collapse text-center text-sm">
        <thead>
          <tr>
            {weeks[0]!.map((d) => (
              <th key={d} scope="col" abbr={weekdayLong().format(utc(d))} className="pb-1 text-[11px] font-medium text-muted-foreground">
                {weekdayShort().format(utc(d)).replace(".", "")}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {weeks.map((week) => (
            <tr key={week[0]}>
              {week.map((date) => {
                const p = parseDate(date)!;
                const outside = p.m !== f.m;
                const selected = date === value;
                const disabled = !!min && date < min;
                return (
                  <td key={date} role="gridcell" aria-selected={selected} className="p-0.5">
                    <button
                      type="button"
                      data-date={date}
                      tabIndex={date === focus ? 0 : -1}
                      disabled={disabled}
                      aria-label={dayLabel().format(utc(date))}
                      aria-current={date === today ? "date" : undefined}
                      onClick={() => onChange(date)}
                      onKeyDown={(e) => onKey(e, date)}
                      onFocus={() => setFocus(date)}
                      className={cn(
                        "grid size-9 place-items-center rounded-md text-sm tabular-nums outline-none transition-colors hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-35",
                        outside && "text-muted-foreground/70",
                        date === today && !selected && "font-semibold text-primary-text ring-1 ring-primary/40",
                        selected && "bg-primary font-semibold text-primary-foreground hover:bg-primary",
                      )}
                    >
                      {p.d}
                    </button>
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
