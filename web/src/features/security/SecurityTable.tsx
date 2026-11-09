// Source: docklet-next/issues/266
// Revision: 5f56262875af9a8ec0fe9007cb889bd3a513085d
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original measured security table, compact filters and labelled card fields; accessible header help in every view.

import type { CSSProperties, ReactNode } from "react";
import { CircleHelp } from "lucide-react";
import { useTranslations } from "use-intl";

import { ColumnFilterRow, ColumnFilterSummary } from "@/shared/components/column-filters";
import { CardSortBar, ROW_EDGES, ROW_SURFACE } from "@/shared/components/overview/overview-table";
import { SortableHead } from "@/shared/components/sortable-head";
import { useFitView } from "@/shared/hooks/use-fit-view";
import type { ColumnFilter, FilterValues } from "@/shared/lib/column-filter";
import { securityColumnGroups } from "./security-layout";
import type { SortState } from "@/shared/lib/table-sort";
import { cn } from "@/shared/lib/utils";
import { Card } from "@/shared/ui/card";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { BareTable, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

export interface SecurityColumn<K extends string> { id: K; label: string; short?: string }
export interface SecurityRow<K extends string> {
  id: string;
  color: string;
  selected?: boolean;
  cells: Record<K, ReactNode>;
}
interface Props<T, K extends string> {
  label: string;
  fitKey: string;
  columns: SecurityColumn<K>[];
  condensed: readonly (readonly K[])[];
  rows: SecurityRow<K>[];
  filters: ColumnFilter<T>[];
  values: FilterValues;
  setValues: (fn: (cur: FilterValues) => FilterValues) => void;
  sort: SortState<K>;
  onSort: (key: K) => void;
  summary: { shown: number; total: number; noun: string; nounOf: string };
  empty: ReactNode;
}

const HEADER_HELP = {
  cvss: "cvss", risk: "risk", epss: "epss", installed: "installed", fixed: "fixed", usage: "usage",
  critical: "count", high: "count", medium: "count", low: "count", unknown: "count",
} as const;

function FieldLabel({ label, help }: { label: string; help?: string }) {
  const t = useTranslations("security");
  return <span className="inline-flex items-center gap-1">
    {label}
    {help && <Tooltip>
      <TooltipTrigger asChild><Button type="button" variant="ghost" size="icon-xs" aria-label={t("headerHelp.label", { column: label })}><CircleHelp aria-hidden /></Button></TooltipTrigger>
      <TooltipContent className="max-w-xs">{help}</TooltipContent>
    </Tooltip>}
  </span>;
}

function CompactFilters<T, K extends string>({ columns, filters, values, onChange }: Pick<Props<T, K>, "columns" | "filters" | "values"> & { onChange: (id: string, value: string) => void }) {
  const t = useTranslations("common.columnFilters");
  return <div data-slot="security-filters" className="grid min-w-0 grid-cols-[repeat(auto-fit,minmax(min(100%,10rem),1fr))] gap-2 rounded-xl bg-muted/45 p-2">
    {columns.map((column) => {
      const filter = filters.find((f) => f.id === column.id);
      if (!filter) return null;
      const value = values[column.id] ?? "";
      const label = t("label", { column: column.label });
      return <label key={column.id} className="flex min-w-0 flex-col gap-1 text-xs text-muted-foreground" data-filtered={!!value || undefined}>
        {column.label}
        {filter.kind === "text" ? <Input type="search" aria-label={label} value={value} placeholder={t("contains")} onChange={(e) => onChange(column.id, e.target.value)} className="h-8 min-w-0 bg-background/60 text-xs" /> :
          <Select value={value || "__all"} onValueChange={(v) => onChange(column.id, v === "__all" ? "" : v)}>
            <SelectTrigger aria-label={label} size="sm" className="w-full min-w-0 bg-background/60 text-xs [&_[data-slot=select-value]]:min-w-0 [&_[data-slot=select-value]]:truncate"><SelectValue /></SelectTrigger>
            <SelectContent>{[{ value: "__all", label: t("all") }, ...filter.options].map((o) => <SelectItem key={o.value} value={o.value} className="[overflow-wrap:anywhere]">{o.label}</SelectItem>)}</SelectContent>
          </Select>}
      </label>;
    })}
  </div>;
}

/** The measured overview policy keeps all columns in cards and merges labelled fields in condensed tables. */
export function SecurityTable<T, K extends string>({ label, fitKey, columns, condensed, rows, filters, values, setValues, sort, onSort, summary, empty }: Props<T, K>) {
  const t = useTranslations("security");
  const tf = useTranslations("common.columnFilters");
  // Filter values only remeasure: a reset would unmount the compact filter being typed into.
  const { view, ref } = useFitView<HTMLElement>(JSON.stringify([fitKey, columns]), undefined, JSON.stringify(values));
  const groups = securityColumnGroups(view, columns.map((c) => c.id), condensed);
  const help = new Map(Object.entries(HEADER_HELP).map(([id, key]) => [id, t(`headerHelp.${key}`, { severity: columns.find((c) => c.id === id)?.label ?? "" })]));
  const onChange = (id: string, value: string) => setValues((cur) => ({ ...cur, [id]: value }));
  const labelled = (row: SecurityRow<K>, ids: readonly K[]) => <dl className="grid min-w-0 gap-2">{ids.map((id) => <div key={id} className="min-w-0" data-field={id}>
    <dt className="text-[11px] text-muted-foreground"><FieldLabel label={columns.find((c) => c.id === id)!.label} help={view === "cards" ? help.get(id) : undefined} /></dt>
    <dd className="min-w-0 text-xs [overflow-wrap:anywhere]">{row.cells[id]}</dd>
  </div>)}</dl>;
  const edge = (row: SecurityRow<K>) => ({ "--host": row.color }) as CSSProperties;
  return <Card className="min-w-0 gap-2 p-4" data-slot="security-table" data-view={view}>
    <ColumnFilterSummary {...summary} values={values} onReset={() => setValues(() => ({}))} className="px-1" />
    {view !== "table" && <>
      <CardSortBar columns={columns.map((c) => ({ ...c, sorts: [{ key: c.id, label: c.label }] }))} sort={sort} onSort={onSort} />
      <CompactFilters columns={columns} filters={filters} values={values} onChange={onChange} />
    </>}
    <div className="min-w-0" data-slot="security-fit">
      {view === "cards" ? <ul ref={ref as React.Ref<HTMLUListElement>} aria-label={label} className="flex min-w-0 flex-col gap-2">
        {!rows.length && <li className="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">{empty}</li>}
        {rows.map((row) => <li key={row.id} data-security-row data-state={row.selected ? "selected" : undefined} style={edge(row)} className="min-w-0 rounded-xl border border-l-[3px] border-border/70 border-l-(--host) bg-card/50 p-3 data-[state=selected]:bg-primary/10">
          {labelled(row, groups.flat())}
        </li>)}
      </ul> : <BareTable ref={ref as React.Ref<HTMLTableElement>} aria-label={label} className="border-separate border-spacing-y-1">
        <TableHeader className="[&_tr]:border-0">
          <TableRow className="hover:bg-transparent">
            {view === "table" ? columns.map((c) => <SortableHead key={c.id} sortKey={c.id} sort={sort} onSort={onSort} help={help.get(c.id)} label={t("filters.sortBy", { column: c.label })} filtered={!!values[c.id]?.trim()} filteredLabel={tf("filtered")}>
              {c.short ? <abbr title={c.label} className="no-underline">{c.short}</abbr> : c.label}
            </SortableHead>) : groups.map((ids) => <TableHead key={ids[0]} className="whitespace-normal"><span className="flex flex-wrap items-center gap-x-2">{ids.map((id) => <FieldLabel key={id} label={columns.find((c) => c.id === id)!.label} help={help.get(id)} />)}</span></TableHead>)}
          </TableRow>
          {view === "table" && <ColumnFilterRow columns={columns} filters={filters} values={values} onChange={onChange} />}
        </TableHeader>
        <TableBody>
          {!rows.length && <TableRow><TableCell colSpan={groups.length} className="rounded-xl border border-dashed py-8 text-center text-muted-foreground">{empty}</TableCell></TableRow>}
          {rows.map((row) => <TableRow key={row.id} data-security-row data-state={row.selected ? "selected" : undefined} style={edge(row)} className={cn("transition-colors duration-(--dur-fast) [&>td]:py-1.5", ROW_SURFACE, ROW_EDGES)}>
            {groups.map((ids) => <TableCell key={ids[0]} className={cn("whitespace-normal text-xs [overflow-wrap:anywhere]", view === "condensed" && "min-w-40")}>
              {view === "table" ? row.cells[ids[0]!] : labelled(row, ids)}
            </TableCell>)}
          </TableRow>)}
        </TableBody>
      </BareTable>}
    </div>
  </Card>;
}
