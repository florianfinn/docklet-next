import { CalendarClock } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import { validateSchedule, type Recurrence, type Schedule } from "@/domain/schedule";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/shared/ui/popover";

import { ScheduleEditor } from "./schedule-editor";
import { useScheduleText } from "./use-schedule-text";

const MODES: readonly Recurrence[] = ["weekly", "daily", "once"];

interface SchedulePopoverProps {
  id: string;
  value: Schedule;
  onApply: (schedule: Schedule) => void;
  /** Accessible context, e.g. the stack name. */
  label: string;
  timeZone: string;
  className?: string;
}

/**
 * Compact schedule cell for tables and cards: the summary with the next run
 * opens the full schedule editor; changes apply only with "Übernehmen" and
 * only when the schedule is valid.
 */
export function SchedulePopover({ id, value, onApply, label, timeZone, className }: SchedulePopoverProps) {
  const t = useTranslations("common.schedule");
  const text = useScheduleText();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<Schedule>(value);
  const [now, setNow] = useState(() => new Date());
  const next = text.next(value, now, timeZone);
  const invalid = !!validateSchedule(draft, now, timeZone);
  return (
    <Popover
      open={open}
      onOpenChange={(o) => {
        setOpen(o);
        if (o) {
          setDraft(value);
          setNow(new Date());
        }
      }}
    >
      <PopoverTrigger asChild>
        <Button id={id} type="button" variant="outline" aria-label={`${t("edit")}: ${label}, ${text.summary(value)}`} data-slot="schedule-trigger" className={cn("h-auto min-h-8 flex-col items-start gap-0 py-1 text-left font-normal whitespace-normal", className)}>
          <span className="flex items-center gap-1.5 text-xs font-medium">
            <CalendarClock className="size-3.5 shrink-0" aria-hidden />
            {text.summary(value)}
          </span>
          <span className="text-[11px] text-muted-foreground">{next ? `${next.relative} · ${next.zone}` : t("noNext", { zone: timeZone })}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-[min(26rem,calc(100vw-2rem))] p-4" aria-label={`${t("edit")}: ${label}`}>
        <div className="flex flex-col gap-4">
          <ScheduleEditor idPrefix={id} value={draft} onChange={(s) => s.mode !== "manual" && setDraft(s)} modes={MODES} timeZone={timeZone} now={now} />
          <div className="flex justify-end gap-2 border-t pt-3">
            <Button type="button" variant="ghost" size="sm" onClick={() => setOpen(false)}>
              {t("cancel")}
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={invalid}
              onClick={() => {
                onApply(draft);
                setOpen(false);
              }}
            >
              {t("apply")}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
