import { useId } from "react";
import { useTranslations } from "use-intl";

import { validateSchedule, weekdayOf, withRecurrence, WEEKDAYS, zonedParts, nextOccurrence, type PlanSchedule, type Weekday } from "@/domain/schedule";
import { cn } from "@/shared/lib/utils";
import { Field, FieldError, FieldLabel } from "@/shared/ui/field";

import { DatePicker } from "../date-time/date-picker";
import { TimePicker } from "../date-time/time-picker";
import { SegmentedToggle } from "../segmented-toggle";
import { useScheduleText } from "./use-schedule-text";

interface ScheduleEditorProps {
  idPrefix: string;
  value: PlanSchedule;
  onChange: (schedule: PlanSchedule) => void;
  /** Offered recurrences, e.g. manual/daily/weekly/once for backups. */
  modes: readonly PlanSchedule["mode"][];
  timeZone: string;
  now: Date;
  className?: string;
}

/**
 * Typed schedule input: recurrence, then date picker, weekday and time picker
 * as separate controls (no free-text window). Weekly keeps its weekday and
 * offers the first date in the calendar; a one-time run needs a date in the
 * future. Shows the next run with its relative time and the hub time zone.
 */
export function ScheduleEditor({ idPrefix, value, onChange, modes, timeZone, now, className }: ScheduleEditorProps) {
  const t = useTranslations("common.schedule");
  const text = useScheduleText();
  const nextId = useId();
  const errId = useId();
  const today = zonedParts(now, timeZone).date;
  const error = value.mode === "manual" ? null : validateSchedule(value, now, timeZone);
  const next = text.next(value, now, timeZone);
  const describedBy = [nextId, error ? errId : ""].filter(Boolean).join(" ");
  const weeklyFirst = value.mode === "weekly" ? ((value.from && value.from > today ? value.from : undefined) ?? (nextOccurrence(value, now, timeZone) ? zonedParts(nextOccurrence(value, now, timeZone)!, timeZone).date : undefined)) : undefined;

  return (
    <div data-slot="schedule-editor" className={cn("flex flex-col gap-3", className)}>
      <Field>
        <FieldLabel id={`${idPrefix}-rec-label`}>{t("recurrence.label")}</FieldLabel>
        <SegmentedToggle<PlanSchedule["mode"]>
          value={value.mode}
          onChange={(mode) => onChange(withRecurrence(value, mode, now, timeZone))}
          label={t("recurrence.label")}
          options={modes.map((m) => ({ value: m, label: m === "manual" ? t("manual") : t(`recurrence.${m}`) }))}
          fit="content"
          wrap
          className="self-start"
        />
      </Field>
      {value.mode !== "manual" && (
        <>
          {value.mode === "weekly" && (
            <Field>
              <FieldLabel>{t("weekday")}</FieldLabel>
              <SegmentedToggle<Weekday>
                value={value.day}
                onChange={(day) => onChange({ ...value, day, from: value.from && weekdayOf(value.from) === day ? value.from : undefined })}
                label={t("weekday")}
                options={WEEKDAYS.map((d) => ({ value: d, label: t(`days.${d}`), ariaLabel: t(`daysLong.${d}`) }))}
                fit="content"
                wrap
                className="self-start"
              />
            </Field>
          )}
          <div className="flex flex-wrap items-end gap-3">
            {(value.mode === "once" || value.mode === "weekly") && (
              <Field className="w-auto">
                <FieldLabel htmlFor={`${idPrefix}-date`}>{value.mode === "once" ? t("date") : t("firstDate")}</FieldLabel>
                <DatePicker
                  id={`${idPrefix}-date`}
                  value={value.mode === "once" ? value.date : weeklyFirst}
                  today={today}
                  min={today}
                  prompt={t("datePrompt")}
                  label={value.mode === "once" ? t("date") : t("firstDate")}
                  invalid={error === "date" || error === "past"}
                  describedBy={describedBy}
                  onChange={(date) => onChange(value.mode === "once" ? { ...value, date } : { ...value, from: date, day: weekdayOf(date) ?? (value as { day: Weekday }).day })}
                />
              </Field>
            )}
            <Field className="w-auto">
              <FieldLabel htmlFor={`${idPrefix}-time`}>{t("time")}</FieldLabel>
              <TimePicker id={`${idPrefix}-time`} value={value.time} onChange={(time) => onChange({ ...value, time })} prompt={t("timePrompt")} label={t("time")} invalid={error === "time"} describedBy={describedBy} />
            </Field>
          </div>
          <p id={nextId} aria-live="polite" data-slot="schedule-next" className="text-xs text-muted-foreground">
            {next ? t("next", { when: next.when, relative: next.relative, zone: next.zone }) : t("noNext", { zone: timeZone })}
          </p>
          {error && <FieldError id={errId}>{t(`errors.${error}`)}</FieldError>}
        </>
      )}
    </div>
  );
}
