import { ArrowDownAZ, ArrowUpZA } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";

import { hostTone } from "@/domain/hosts";
import type { Host } from "@/domain/model";
import { useFitView } from "@/shared/hooks/use-fit-view";
import type { FitView } from "@/shared/lib/fit-view";
import type { SortDir, SortState } from "@/shared/lib/table-sort";
import { cappedGrid } from "@/shared/lib/host-block-grid";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";

import { CardSortBar, OverviewView, type OverviewColumn, type TableView } from "./overview/overview-table";
import { ToneBadge, ToneDot } from "./tone";

export interface HostBlockLabels {
  region: string;
  order: string;
  asc: string;
  desc: string;
  empty: string;
  status: (h: Host) => string;
}

interface HostBlocksProps<T, K extends string> {
  /** Visible hosts in name order; the block order follows hostDir. */
  hosts: Host[];
  /** Filtered and sorted rows; each block keeps their relative order. */
  rows: T[];
  hostOf: (row: T) => string;
  hostDir: SortDir;
  onHostDir: (dir: SortDir) => void;
  labels: HostBlockLabels;
  summary: (host: Host, rows: T[]) => ReactNode;
  /** Hide blocks without rows, e.g. while a search or status filter is active. */
  hideEmpty?: boolean;
  /**
   * Narrowest column of the block grid for the current mode, e.g. "34rem".
   * All blocks share one column width; a table that does not fit it switches
   * to its condensed table and then to cards.
   */
  minWidth: string;
  /**
   * Optional density limits: at most this many blocks per row even on 21:9
   * screens, and no block wider than `maxBlockWidth`, so one host or a filter
   * never stretches a block over the whole page. Without them the grid fills
   * the row as before.
   */
  maxColumns?: number;
  maxBlockWidth?: string;
  /** Mode and similar inputs; a change measures every block again. */
  fitKey: string;
  columns: (view: TableView) => OverviewColumn<K>[];
  sort: SortState<K>;
  onSort: (key: K) => void;
  sortLabel: (column: string) => string;
  sortExtra?: ReactNode;
  /**
   * Where card-view blocks offer sorting: one shared bar above the blocks
   * ("top"), or a compact control in each card block's header ("header"),
   * for pages whose table blocks already sort through column headers.
   */
  sortBar?: "top" | "header";
  selecting: boolean;
  selectFor: (host: Host, rows: T[]) => ReactNode;
  actionsLabel: string;
  dense: boolean;
  /** Rows hidden from the overview; shown collapsed at the bottom of their host's block. */
  hidden?: T[];
  renderHidden?: (host: Host, rows: T[]) => ReactNode;
  /** Live load sort: rows reorder every tick and must not animate. */
  liveSort?: boolean;
  children: (host: Host, rows: T[], view: FitView, columns: OverviewColumn<K>[]) => ReactNode;
}

/**
 * Host blocks on one responsive grid without horizontal scrolling. Every
 * column has the same width at a given screen, also in the last row; the
 * number of columns follows the available width and the mode's minimum. Each
 * block measures its own table and falls back to the condensed table or
 * cards when its column is too narrow. Filters, sort and selection are the
 * page's own.
 */
export function HostBlocks<T, K extends string>(props: HostBlocksProps<T, K>) {
  const { hosts, rows, hostOf, hostDir, onHostDir, labels, hideEmpty, minWidth, hidden = [] } = props;
  const [cardBlocks, setCardBlocks] = useState<Record<string, boolean>>({});
  const ordered = hostDir === "asc" ? hosts : hosts.toReversed();
  const blocks = ordered
    .map((h) => ({ host: h, rows: rows.filter((r) => hostOf(r) === h.id), hidden: hidden.filter((r) => hostOf(r) === h.id) }))
    .filter((b) => !hideEmpty || b.rows.length || b.hidden.length);
  const anyCards = (props.sortBar ?? "top") === "top" && blocks.some((b) => cardBlocks[b.host.id]);
  return (
    <div className="flex flex-col gap-2" data-slot="host-blocks" data-live-sort={props.liveSort || undefined}>
      <div className="flex flex-wrap items-center justify-end gap-2">
        {anyCards && <CardSortBar columns={props.columns("table")} sort={props.sort} onSort={props.onSort} extra={props.sortExtra} />}
        <span className="flex-1" />
        <Button variant="ghost" size="sm" className="h-7 text-xs text-muted-foreground" onClick={() => onHostDir(hostDir === "asc" ? "desc" : "asc")} aria-label={`${labels.order}: ${hostDir === "asc" ? labels.asc : labels.desc}`}>
          {hostDir === "asc" ? <ArrowDownAZ data-icon="inline-start" /> : <ArrowUpZA data-icon="inline-start" />}
          {labels.order}: {hostDir === "asc" ? labels.asc : labels.desc}
        </Button>
      </div>
      <div
        role="region"
        aria-label={labels.region}
        data-slot="host-block-grid"
        data-max-columns={props.maxColumns}
        className={cn("grid w-full items-start gap-4", !props.maxColumns && "[grid-template-columns:repeat(auto-fit,minmax(min(100%,var(--hb-min)),1fr))]")}
        style={{ "--hb-min": minWidth, ...(props.maxColumns ? cappedGrid(props.maxColumns, blocks.length, props.maxBlockWidth) : {}) } as React.CSSProperties}
      >
        {blocks.map(({ host, rows: own, hidden: ownHidden }) => (
          <HostBlock key={host.id} {...props} host={host} rows={own} hiddenRows={ownHidden} onCards={(on) => setCardBlocks((s) => (!!s[host.id] === on ? s : { ...s, [host.id]: on }))} />
        ))}
      </div>
    </div>
  );
}

function HostBlock<T, K extends string>({ host, rows, hiddenRows, onCards, ...p }: HostBlocksProps<T, K> & { host: Host; rows: T[]; hiddenRows: T[]; onCards: (on: boolean) => void }) {
  const { view, ref } = useFitView<HTMLElement>(`${p.fitKey}|${rows.length}`);
  useEffect(() => onCards(view === "cards"), [view, onCards]);
  const columns = p.columns(view === "cards" ? "table" : view);
  return (
    <section
      data-host-block={host.id}
      data-view={view}
      data-glow
      aria-labelledby={`hb-${host.id}`}
      style={{ "--host": host.color } as React.CSSProperties}
      className="min-w-0 overflow-clip rounded-2xl border border-(--host)/35 bg-card/60"
    >
      <header className="flex flex-wrap items-center gap-x-2.5 gap-y-1 border-b border-(--host)/30 bg-(--host)/10 px-3.5 py-2.5">
        <span className="size-2.5 shrink-0 rounded-[3px] bg-(--host)" aria-hidden />
        <h3 id={`hb-${host.id}`} className="font-semibold tracking-tight">
          {host.name}
        </h3>
        {host.status !== "online" && (
          <ToneBadge tone={hostTone(host)} className="h-5 px-1.5 text-[10.5px]">
            <ToneDot tone={hostTone(host)} pulse className="size-1.5" />
            {p.labels.status(host)}
          </ToneBadge>
        )}
        <span className="ml-auto text-xs text-muted-foreground tabular-nums">{p.summary(host, rows)}</span>
        {p.sortBar === "header" && view === "cards" && (
          <div className="flex w-full justify-end" data-slot="host-block-sort">
            <CardSortBar compact label={host.name} columns={p.columns("table")} sort={p.sort} onSort={p.onSort} extra={p.sortExtra} />
          </div>
        )}
      </header>
      {rows.length ? (
        <div className="min-w-0 px-1.5 py-1">
          <OverviewView
            view={view}
            columns={columns}
            sort={p.sort}
            onSort={p.onSort}
            sortLabel={p.sortLabel}
            selecting={p.selecting}
            select={p.selectFor(host, rows)}
            actionsLabel={p.actionsLabel}
            dense={p.dense}
            sortBar={false}
            listLabel={host.name}
            innerRef={ref}
          >
            {p.children(host, rows, view, columns)}
          </OverviewView>
        </div>
      ) : (
        <p className="px-3.5 py-4 text-xs text-muted-foreground">{p.labels.empty}</p>
      )}
      {hiddenRows.length > 0 && p.renderHidden && <div className="pb-1.5">{p.renderHidden(host, hiddenRows)}</div>}
    </section>
  );
}
