import { useId } from "react";
import { useTranslations } from "use-intl";

import { RETENTION_LIMITS, validateRetention, type Retention } from "@/domain/retention";
import { Field, FieldContent, FieldDescription, FieldError, FieldLabel, FieldLegend, FieldSet } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";
import { Slider } from "@/shared/ui/slider";
import { Switch } from "@/shared/ui/switch";

interface RetentionEditorProps {
  idPrefix: string;
  value: Retention;
  onChange: (value: Retention) => void;
  legend: string;
  className?: string;
}

type Limit = "count" | "size";

/**
 * Two independent retention limits: archive count and total size. Each has
 * its own switch, a slider for quick choices and a number field for exact or
 * larger values; the summary states how both combine, including "unlimited".
 */
export function RetentionEditor({ idPrefix, value, onChange, legend, className }: RetentionEditorProps) {
  const t = useTranslations("common.retention");
  const summaryId = useId();
  const error = validateRetention(value);
  const key = (limit: Limit) => (limit === "count" ? "maxCount" : "maxSizeGb");
  const toggle = (limit: Limit, on: boolean) => onChange({ ...value, [key(limit)]: on ? RETENTION_LIMITS[limit].fallback : null });
  const set = (limit: Limit, n: number) => onChange({ ...value, [key(limit)]: n });
  const summary =
    value.maxCount !== null && value.maxSizeGb !== null
      ? t("summary.both", { count: value.maxCount, size: value.maxSizeGb })
      : value.maxCount !== null
        ? t("summary.count", { count: value.maxCount })
        : value.maxSizeGb !== null
          ? t("summary.size", { size: value.maxSizeGb })
          : t("summary.none");

  const row = (limit: Limit) => {
    const current = value[key(limit)];
    const on = current !== null;
    const lim = RETENTION_LIMITS[limit];
    const id = `${idPrefix}-${limit}`;
    const invalid = on && error === limit;
    return (
      <div className="flex flex-col gap-2 rounded-xl border bg-card/40 p-3" data-retention={limit}>
        <Field orientation="horizontal">
          <Switch id={`${id}-on`} checked={on} onCheckedChange={(v) => toggle(limit, v)} aria-describedby={summaryId} />
          <FieldContent>
            <FieldLabel htmlFor={`${id}-on`}>{t(`${limit}.switch`)}</FieldLabel>
            <FieldDescription>{t(`${limit}.hint`)}</FieldDescription>
          </FieldContent>
        </Field>
        {on && (
          <div className="flex flex-wrap items-center gap-3 pl-11">
            <Slider
              value={[Math.min(lim.slider, Math.max(lim.min, Number.isFinite(current) ? current : lim.min))]}
              min={lim.min}
              max={lim.slider}
              step={1}
              onValueChange={([v]) => v !== undefined && set(limit, v)}
              aria-label={t(`${limit}.slider`)}
              aria-valuetext={t(`${limit}.value`, { value: current })}
              className="min-w-40 flex-1"
            />
            <span className="flex items-center gap-1.5">
              <Input
                id={`${id}-value`}
                type="number"
                inputMode="numeric"
                min={lim.min}
                max={lim.max}
                step={1}
                value={Number.isFinite(current) ? current : ""}
                onChange={(e) => set(limit, e.target.value === "" ? Number.NaN : Number(e.target.value))}
                aria-label={t(`${limit}.input`)}
                aria-invalid={invalid || undefined}
                aria-describedby={invalid ? `${id}-err` : undefined}
                className="h-8 w-24 font-mono text-xs tabular-nums"
              />
              <span className="text-xs text-muted-foreground">{t(`${limit}.unit`, { value: Number.isFinite(current) ? current : 0 })}</span>
            </span>
            {invalid && (
              <FieldError id={`${id}-err`} className="basis-full">
                {t(`${limit}.error`, { min: lim.min, max: lim.max })}
              </FieldError>
            )}
          </div>
        )}
      </div>
    );
  };

  return (
    <FieldSet className={className} data-slot="retention-editor">
      <FieldLegend variant="label">{legend}</FieldLegend>
      {row("count")}
      {row("size")}
      <p id={summaryId} aria-live="polite" data-slot="retention-summary" className="text-xs text-muted-foreground">
        {summary}
      </p>
    </FieldSet>
  );
}
