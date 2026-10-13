import { ArrowDownWideNarrow, ArrowUpNarrowWide } from "lucide-react";
import type { MouseEvent, ReactNode, Ref } from "react";
import { useTranslations } from "use-intl";

import { useFitView } from "@/shared/hooks/use-fit-view";
import type { FitView } from "@/shared/lib/fit-view";
import { ariaSort, type SortState } from "@/shared/lib/table-sort";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { BareTable, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table";

import { ActionsContextMenu, type MenuGroups } from "../action-menu";
import { SortButton } from "../sortable-head";

/** Table layouts; cards reuse the full table's columns. */
export type TableView = Exclude<FitView, "cards">;

export interface OverviewColumn<K extends string = string> {
  /** "name" is the primary cell: first column in tables, card title in cards. */
  id: string;
  label: string;
  /** Sort keys reachable from this header, in order; several keys share one merged header. */
  sorts?: readonly { key: K; label: string }[];
  /** Custom header content, e.g. the load header with its metric choice. */
  head?: ReactNode;
  className?: string;
  /** Card view: the field takes a whole row (fixed-width or long content such as load rings or chip lists). */
  wide?: boolean;
}

interface HeadProps<K extends string> {
  columns: OverviewColumn<K>[];
  /** Optional: lists without sortable columns (history, settings tables) omit it. */
  sort?: SortState<K>;
  onSort?: (key: K) => void;
  sortLabel?: (column: string) => string;
  selecting: boolean;
  select?: ReactNode;
  actionsLabel: string;
}

/** Column headers; every sortable key is a button, aria-sort marks the column holding the active key. */
export function OverviewHead<K extends string>({ columns, sort = { key: "" as K, dir: "asc" }, onSort = () => {}, sortLabel = (c) => c, selecting, select, actionsLabel }: HeadProps<K>) {
  return (
    <TableHeader className="[&_tr]:border-0">
      <TableRow className="hover:bg-transparent">
        {selecting && <TableHead className="w-10 pl-3">{select}</TableHead>}
        {columns.map((col) => {
          const keys = col.sorts?.map((s) => s.key) ?? [];
          if (!keys.length && !col.head)
            return (
              <TableHead key={col.id} className={col.className}>
                {col.label}
              </TableHead>
            );
          return (
            <TableHead key={col.id} aria-sort={keys.length ? ariaSort(sort, keys) : undefined} data-sorted={keys.includes(sort.key) || undefined} className={cn("px-1", col.className)}>
              {col.head ?? (
                <span className="flex flex-wrap items-center gap-0.5">
                  {col.sorts!.map((s) => (
                    <SortButton key={s.key} sortKey={s.key} sort={sort} onSort={onSort} label={sortLabel(s.label)}>
                      {s.label}
                    </SortButton>
                  ))}
                </span>
              )}
            </TableHead>
          );
        })}
        <TableHead className="w-12">
          <span className="sr-only">{actionsLabel}</span>
        </TableHead>
      </TableRow>
    </TableHeader>
  );
}

/** Row look of the overview tables (card-like cells with an edge in --host); shared with other separated-row tables. */
export const ROW_SURFACE = "[&>td]:border-y [&>td]:border-border/70 [&>td]:bg-card/50 hover:[&>td]:bg-muted/45 data-[state=selected]:[&>td]:bg-primary/10";
export const ROW_EDGES = "[&>td:first-child]:rounded-l-xl [&>td:first-child]:border-l-[3px] [&>td:first-child]:border-l-(--host) [&>td:last-child]:rounded-r-xl [&>td:last-child]:border-r";

interface RowViewProps {
  view: FitView;
  columns: OverviewColumn[];
  cells: Partial<Record<string, ReactNode>>;
  /** Selection checkbox; only passed in Advanced. */
  select?: ReactNode;
  actions: ReactNode;
  selected?: boolean;
  busy?: boolean;
  hostColor?: string;
  /** Same entries as the ellipsis button; right click on the row or card opens them. */
  menu?: MenuGroups;
  menuTitle?: string;
  /** Vertical cell padding of the table row. */
  pad: string;
  onClick?: (e: MouseEvent<HTMLElement>) => void;
  className?: string;
}

/**
 * One overview entry as table row or stacked card. Both show the same cells:
 * in cards every column becomes a labelled field, so Advanced keeps all its
 * data and the selection checkbox. The whole row or card is clickable; nested
 * controls keep their own behaviour.
 */
export function OverviewRowView({ view, columns, cells, select, actions, selected, busy, hostColor, menu, menuTitle = "", pad, onClick, className }: RowViewProps) {
  const style = { "--host": hostColor ?? "var(--border)" } as React.CSSProperties;
  const state = selected ? "selected" : undefined;
  if (view === "cards") {
    return (
      <ActionsContextMenu groups={menu} title={menuTitle}>
      <li
        data-overview-card
        data-state={state}
        onClick={onClick}
        style={style}
        className={cn(
          "group/row cursor-pointer rounded-xl border border-l-[3px] border-border/70 border-l-(--host) bg-card/50 p-3 transition-colors duration-(--dur-fast) hover:bg-muted/45 data-[state=selected]:bg-primary/10",
          busy && "bg-primary/6",
          className,
        )}
      >
        <div className="flex items-start gap-2.5">
          {select && <div className="pt-1">{select}</div>}
          <div className="min-w-0 flex-1">{cells.name}</div>
          <div className="-mt-1 -mr-1 shrink-0">{actions}</div>
        </div>
        <dl className="mt-2.5 grid grid-cols-[repeat(auto-fill,minmax(min(100%,8rem),1fr))] gap-x-3 gap-y-2">
          {columns
            .filter((c) => c.id !== "name")
            .map((c) => (
              <div key={c.id} className={cn("min-w-0", c.wide && "col-span-full")}>
                <dt className="text-[11px] text-muted-foreground">{c.label}</dt>
                <dd className="mt-0.5 min-w-0 text-xs [overflow-wrap:anywhere]">{cells[c.id] ?? "–"}</dd>
              </div>
            ))}
        </dl>
      </li>
      </ActionsContextMenu>
    );
  }
  return (
    <ActionsContextMenu groups={menu} title={menuTitle}>
    <TableRow onClick={onClick} data-state={state} style={style} className={cn("group/row cursor-pointer border-0 transition-colors duration-(--dur-fast)", ROW_SURFACE, ROW_EDGES, busy && "[&>td]:bg-primary/6", className)}>
      {select && <TableCell className={cn(pad, "w-10 pl-3")}>{select}</TableCell>}
      {columns.map((c) => (
        <TableCell key={c.id} className={cn(pad, "whitespace-normal", c.className)}>
          {cells[c.id]}
        </TableCell>
      ))}
      <TableCell className={cn(pad, "w-12 pr-2 text-right")}>{actions}</TableCell>
    </TableRow>
    </ActionsContextMenu>
  );
}

interface SortBarProps<K extends string> {
  columns: OverviewColumn<K>[];
  sort?: SortState<K>;
  onSort?: (key: K) => void;
  /** Shown next to the key choice, e.g. the CPU/RAM choice of the load sort. */
  extra?: ReactNode;
  select?: ReactNode;
  /** Smaller controls for a block header; the label names the block for screen readers. */
  compact?: boolean;
  label?: string;
}

/** Sorting for the card view, where no column headers exist: key choice and direction. */
export function CardSortBar<K extends string>({ columns, sort, onSort, extra, select, compact, label }: SortBarProps<K>) {
  const t = useTranslations("common.overview");
  const own = columns.flatMap((c) => c.sorts ?? []);
  if (!own.length || !sort || !onSort) return null;
  // Inside host blocks the host column is absent; the default host order still needs a visible name.
  const options = own.some((o) => o.key === sort.key) ? own : [{ key: sort.key, label: t("defaultOrder") }, ...own];
  return (
    <div data-slot="card-sort-bar" data-compact={compact || undefined} className={cn("flex flex-wrap items-center px-0.5", compact ? "gap-1.5" : "gap-2")}>
      {select && <span className="flex items-center gap-2 pr-1 text-xs text-muted-foreground">{select}</span>}
      <Select value={sort.key} onValueChange={(k) => k !== sort.key && onSort(k as K)}>
        <SelectTrigger size="sm" className={cn("w-auto", compact ? "h-7 min-w-32 text-xs" : "h-8 min-w-40")} aria-label={label ? `${t("sortKey")} (${label})` : t("sortKey")}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {options.map((o) => (
              <SelectItem key={o.key} value={o.key}>
                {o.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <Button type="button" variant="outline" size="sm" className={compact ? "h-7 text-xs" : "h-8"} onClick={() => onSort(sort.key)} aria-label={`${t("directionToggle")}: ${t(`direction.${sort.dir}`)}${label ? ` (${label})` : ""}`}>
        {sort.dir === "asc" ? <ArrowUpNarrowWide data-icon="inline-start" /> : <ArrowDownWideNarrow data-icon="inline-start" />}
        {t(`direction.${sort.dir}`)}
      </Button>
      {extra}
    </div>
  );
}

export interface OverviewViewProps<K extends string> extends HeadProps<K> {
  view: FitView;
  dense: boolean;
  /** Rows (table) or cards for the given view and columns. */
  children: ReactNode;
  /** Card view only: show the sort bar (host blocks show one shared bar instead). */
  sortBar?: boolean;
  sortExtra?: ReactNode;
  listLabel?: string;
  innerRef?: Ref<HTMLElement>;
}

/** Renders one view; the caller measured which one fits. */
export function OverviewView<K extends string>({ view, dense, children, sortBar = true, sortExtra, listLabel, innerRef, ...head }: OverviewViewProps<K>) {
  if (view === "cards") {
    return (
      <div ref={innerRef as Ref<HTMLDivElement>} data-view="cards" className="flex min-w-0 flex-col gap-2">
        {sortBar && <CardSortBar columns={head.columns} sort={head.sort} onSort={head.onSort} extra={sortExtra} select={head.selecting ? head.select : undefined} />}
        <ul aria-label={listLabel} className="flex flex-col gap-2">
          {children}
        </ul>
      </div>
    );
  }
  return (
    <BareTable ref={innerRef as Ref<HTMLTableElement>} data-view={view} className={dense ? "border-separate border-spacing-y-1" : "border-separate border-spacing-y-1.5"}>
      <OverviewHead {...head} />
      <TableBody>{children}</TableBody>
    </BareTable>
  );
}

interface OverviewTableProps<K extends string> extends Omit<HeadProps<K>, "columns"> {
  /** New key = measure again from the widest view (mode, rows, …). */
  fitKey: string;
  columns: (view: TableView) => OverviewColumn<K>[];
  dense: boolean;
  sortExtra?: ReactNode;
  listLabel?: string;
  /** Live load sort: rows reorder every tick and must not animate. */
  liveSort?: boolean;
  children: (view: FitView, columns: OverviewColumn<K>[]) => ReactNode;
}

/**
 * Single overview list without horizontal scrolling: the full table while it
 * fits, then the condensed table, then cards. The choice is measured on the
 * real layout (see useFitView), so long names or wide chips are accounted for.
 */
export function OverviewTable<K extends string>({ fitKey, columns, children, liveSort, ...rest }: OverviewTableProps<K>) {
  const { view, ref } = useFitView<HTMLElement>(fitKey);
  const cols = columns(view === "cards" ? "table" : view);
  return (
    <div data-slot="overview-fit" data-view={view} data-live-sort={liveSort || undefined} className="min-w-0">
      <OverviewView view={view} columns={cols} innerRef={ref} {...rest}>
        {children(view, cols)}
      </OverviewView>
    </div>
  );
}
