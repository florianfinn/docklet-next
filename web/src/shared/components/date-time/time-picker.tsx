import { Clock } from "lucide-react";
import { useRef, useState, type KeyboardEvent } from "react";
import { useTranslations } from "use-intl";

import { isTime } from "@/domain/schedule";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/shared/ui/popover";

interface TimePickerProps {
  id: string;
  /** HH:MM (24 h) or empty. */
  value?: string;
  onChange: (time: string) => void;
  prompt: string;
  label: string;
  invalid?: boolean;
  describedBy?: string;
  disabled?: boolean;
  /** Minute step of the list; a stored minute outside the step stays selectable. */
  step?: number;
  className?: string;
}

const pad = (n: number) => String(n).padStart(2, "0");

/** Radio group of time parts in a grid; arrows move within it, Home/End jump to the ends. */
function PartGrid({ label, options, value, columns, onPick }: { label: string; options: string[]; value?: string; columns: number; onPick: (v: string) => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const current = Math.max(0, options.indexOf(value ?? ""));
  const onKey = (e: KeyboardEvent<HTMLButtonElement>, i: number) => {
    const delta = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -columns, ArrowDown: columns }[e.key];
    const target = e.key === "Home" ? 0 : e.key === "End" ? options.length - 1 : delta !== undefined ? Math.min(options.length - 1, Math.max(0, i + delta)) : undefined;
    if (target === undefined) return;
    e.preventDefault();
    ref.current?.querySelectorAll<HTMLButtonElement>("button")[target]?.focus();
  };
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-[11px] font-medium text-muted-foreground">{label}</span>
      <div ref={ref} role="radiogroup" aria-label={label} className="grid gap-1" style={{ gridTemplateColumns: `repeat(${columns}, minmax(2.25rem, 1fr))` }}>
        {options.map((o, i) => {
          const on = o === value;
          return (
            <button
              key={o}
              type="button"
              role="radio"
              aria-checked={on}
              tabIndex={i === current ? 0 : -1}
              onClick={() => onPick(o)}
              onKeyDown={(e) => onKey(e, i)}
              className={cn("h-8 rounded-md font-mono text-xs tabular-nums outline-none transition-colors hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring", on && "bg-primary font-semibold text-primary-foreground hover:bg-primary")}
            >
              {o}
            </button>
          );
        })}
      </div>
    </div>
  );
}

/**
 * Button with the chosen time (or a prompt) that opens hour and minute
 * choices. Picking the hour keeps the popup open, picking the minute closes it.
 */
export function TimePicker({ id, value, onChange, prompt, label, invalid, describedBy, disabled, step = 5, className }: TimePickerProps) {
  const t = useTranslations("common.schedule.timePicker");
  const [open, setOpen] = useState(false);
  const contentRef = useRef<HTMLDivElement>(null);
  const chosen = value && isTime(value) ? value : undefined;
  const [h, m] = chosen ? chosen.split(":") : [undefined, undefined];
  const minutes = Array.from({ length: Math.ceil(60 / step) }, (_, i) => pad(i * step));
  if (m && !minutes.includes(m)) minutes.splice(minutes.findIndex((x) => x > m) === -1 ? minutes.length : minutes.findIndex((x) => x > m), 0, m);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          disabled={disabled}
          aria-label={chosen ? `${label}: ${chosen}` : `${label}: ${prompt}`}
          aria-invalid={invalid || undefined}
          aria-describedby={describedBy}
          data-slot="time-picker"
          className={cn("w-32 justify-start font-normal tabular-nums", !chosen && "text-muted-foreground", className)}
        >
          <Clock data-icon="inline-start" />
          {chosen ?? prompt}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="flex w-auto flex-col gap-3 p-3"
        ref={contentRef}
        onOpenAutoFocus={(e) => {
          e.preventDefault();
          contentRef.current?.querySelector<HTMLButtonElement>('[role="radio"][tabindex="0"]')?.focus();
        }}
      >
        <PartGrid label={t("hours")} options={Array.from({ length: 24 }, (_, i) => pad(i))} value={h} columns={6} onPick={(hour) => onChange(`${hour}:${m ?? "00"}`)} />
        <PartGrid
          label={t("minutes")}
          options={minutes}
          value={m}
          columns={6}
          onPick={(minute) => {
            onChange(`${h ?? "03"}:${minute}`);
            setOpen(false);
          }}
        />
      </PopoverContent>
    </Popover>
  );
}
