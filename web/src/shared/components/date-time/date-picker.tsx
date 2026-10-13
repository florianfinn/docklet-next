import { CalendarDays } from "lucide-react";
import { useRef, useState } from "react";

import { intlTag } from "@/domain/locale";
import { parseDate } from "@/domain/schedule";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/shared/ui/popover";

import { Calendar } from "./calendar";

interface DatePickerProps {
  id: string;
  value?: string;
  onChange: (date: string) => void;
  today: string;
  min?: string;
  /** Visible prompt while no date is chosen, e.g. "Datum wählen". */
  prompt: string;
  /** Accessible name of the trigger and the calendar, e.g. "Datum". */
  label: string;
  invalid?: boolean;
  describedBy?: string;
  disabled?: boolean;
  className?: string;
}


function formatPickedDate(date: string): string {
  const p = parseDate(date);
  return p ? new Intl.DateTimeFormat(intlTag(), { weekday: "short", day: "2-digit", month: "2-digit", year: "numeric", timeZone: "UTC" }).format(new Date(Date.UTC(p.y, p.m - 1, p.d))) : date;
}

/** Button with the chosen date (or a prompt) that opens the calendar; choosing a day closes it. */
export function DatePicker({ id, value, onChange, today, min, prompt, label, invalid, describedBy, disabled, className }: DatePickerProps) {
  const [open, setOpen] = useState(false);
  const contentRef = useRef<HTMLDivElement>(null);
  const chosen = value && parseDate(value) ? value : undefined;
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          disabled={disabled}
          aria-label={chosen ? `${label}: ${formatPickedDate(chosen)}` : `${label}: ${prompt}`}
          aria-invalid={invalid || undefined}
          aria-describedby={describedBy}
          data-slot="date-picker"
          className={cn("w-44 justify-start font-normal tabular-nums", !chosen && "text-muted-foreground", className)}
        >
          <CalendarDays data-icon="inline-start" />
          {chosen ? formatPickedDate(chosen) : prompt}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-auto p-3"
        ref={contentRef}
        onOpenAutoFocus={(e) => {
          e.preventDefault();
          contentRef.current?.querySelector<HTMLButtonElement>('[data-date][tabindex="0"]')?.focus();
        }}
      >
        <Calendar
          value={chosen}
          today={today}
          min={min}
          label={label}
          onChange={(date) => {
            onChange(date);
            setOpen(false);
          }}
        />
      </PopoverContent>
    </Popover>
  );
}
