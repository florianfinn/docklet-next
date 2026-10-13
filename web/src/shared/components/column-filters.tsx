import { FilterX, Search, X } from "lucide-react";
import { useTranslations } from "use-intl";

import type { ColumnFilter, FilterValues } from "@/shared/lib/column-filter";
import { activeFilters } from "@/shared/lib/column-filter";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/shared/ui/input-group";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { TableHead, TableRow } from "@/shared/ui/table";

const ALL = "__all";

/** Set filters take the selected-control look of the segmented filters: primary edge, tint and readable primary text. */
const ACTIVE = "border-control/55 bg-control/10 text-control-text dark:bg-control/15";

/**
 * Second header row with one filter per column, in column order. `columns`
 * lists every column id with its visible label; columns without a filter get
 * an empty cell, so the controls stay under their headings. The row is one
 * recessed surface with rounded ends; tables using it should separate rows
 * (border-separate) so the surface stands apart from the header.
 */
export function ColumnFilterRow<T>({ columns, filters, values, onChange }: { columns: { id: string; label: string }[]; filters: readonly ColumnFilter<T>[]; values: FilterValues; onChange: (id: string, value: string) => void }) {
  const t = useTranslations("common.columnFilters");
  const byId = new Map(filters.map((f) => [f.id, f]));
  return (
    <TableRow data-slot="column-filters" className="border-0 hover:bg-transparent [&>th]:bg-muted/45 [&>th:first-child]:rounded-l-xl [&>th:last-child]:rounded-r-xl dark:[&>th]:bg-muted/30">
      {columns.map((c) => {
        const f = byId.get(c.id);
        const value = values[c.id] ?? "";
        const label = t("label", { column: c.label });
        return (
          <TableHead key={c.id} data-filtered={value ? true : undefined} className="h-auto px-1 py-1.5 font-normal first:pl-1.5 last:pr-1.5">
            {!f ? null : f.kind === "text" ? (
              <InputGroup className={cn("h-8 min-w-28 bg-background/60 dark:bg-input/30", value && ACTIVE)}>
                <InputGroupAddon className="pl-2">
                  <Search className={cn("size-3.5", value && "text-control-text")} />
                </InputGroupAddon>
                <InputGroupInput type="search" value={value} onChange={(e) => onChange(c.id, e.target.value)} placeholder={t("contains")} aria-label={label} className="h-full px-1.5 text-xs md:text-xs [&::-webkit-search-cancel-button]:hidden" />
                {value && (
                  <InputGroupAddon align="inline-end" className="pr-1">
                    <InputGroupButton size="icon-xs" aria-label={t("clear", { column: c.label })} onClick={() => onChange(c.id, "")} className="text-control-text hover:bg-control/15">
                      <X />
                    </InputGroupButton>
                  </InputGroupAddon>
                )}
              </InputGroup>
            ) : (
              <Select value={value || ALL} onValueChange={(v) => onChange(c.id, v === ALL ? "" : v)}>
                <SelectTrigger size="sm" aria-label={label} className={cn("w-full min-w-20 gap-1 bg-background/60 px-2 text-xs font-normal text-muted-foreground data-[size=sm]:h-8 dark:bg-input/30", value && cn(ACTIVE, "font-medium [&_svg:not([class*='text-'])]:text-control-text"))}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value={ALL}>{t("all")}</SelectItem>
                    {f.options.map((o) => (
                      <SelectItem key={o.value} value={o.value}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            )}
          </TableHead>
        );
      })}
    </TableRow>
  );
}

/** Count of shown rows and a reset while any column filter is set. */
/** `noun` follows a plain count ("13 Befunde"), `nounOf` the filtered one ("3 von 13 Befunden"). */
export function ColumnFilterSummary({ shown, total, values, onReset, noun, nounOf, className }: { shown: number; total: number; values: FilterValues; onReset: () => void; noun: string; nounOf: string; className?: string }) {
  const t = useTranslations("common.columnFilters");
  const n = activeFilters(values);
  return (
    <div className={cn("flex min-h-8 flex-wrap items-center gap-2 text-xs text-muted-foreground", className)} data-slot="column-filter-summary" data-active={n > 0 || undefined}>
      <span aria-live="polite" className={cn("tabular-nums", n > 0 && "text-foreground")}>
        {n ? t("countFiltered", { shown, total, noun: nounOf, filters: n }) : t("count", { total, noun })}
      </span>
      {n > 0 && (
        <Button type="button" variant="outline" size="sm" className="text-xs" onClick={onReset}>
          <FilterX data-icon="inline-start" />
          {t("reset")}
        </Button>
      )}
    </div>
  );
}
