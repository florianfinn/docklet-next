// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original availability and resource sections for issues #335 and #336. M19: total contract-code catalogue maps.

import { HISTORY_STATE_KEYS } from "@/data/wire-catalogue-states";
import { scopedKey } from "@/data/wire-catalogue";
import type { UseQueryResult } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { useTranslations } from "use-intl";

import { useContainerHistory, useStackHistory } from "@/data/queries";
import { formatBytes, formatDateTime, formatNumber, formatPercent, formatTime } from "@/domain/format";
import { BUCKET_TONE, HISTORY_RANGES, type ContainerHistoryRef, type HistoryBucket, type HistoryRange, type RuntimeHistory } from "@/domain/history";
import { QueryError } from "@/shared/components/page";
import { cn } from "@/shared/lib/utils";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { Skeleton } from "@/shared/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/shared/ui/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

const RANGE_KEY = { "24h": "r24h", "7d": "r7d", "30d": "r30d" } as const;

/** History of one container; it follows the container name across recreations. */
export function ContainerHistory({ hostId, name }: ContainerHistoryRef) {
  const [range, setRange] = useState<HistoryRange>("24h");
  return <HistorySection range={range} onRange={setRange} query={useContainerHistory({ hostId, name }, range)} />;
}

/** History of one stack: each slot shows the worst state of its services. */
export function StackHistory({ stackId }: { stackId: string }) {
  const [range, setRange] = useState<HistoryRange>("24h");
  return <HistorySection range={range} onRange={setRange} query={useStackHistory(stackId, range)} />;
}

export function HistorySection({ range, onRange, query }: { range: HistoryRange; onRange: (r: HistoryRange) => void; query: UseQueryResult<RuntimeHistory> }) {
  const t = useTranslations("history");
  const h = query.data;
  return (
    <div className="grid min-w-0 gap-4 lg:grid-cols-2" data-history>
      <Card className="min-w-0 gap-3 py-4">
        <CardHeader className="flex flex-row flex-wrap items-center gap-2 px-4">
          <CardTitle className="text-sm">{t("availability")}</CardTitle>
          {h && <Uptime value={h.uptimePercent} />}
          <span className="flex-1" />
          <ToggleGroup type="single" size="sm" variant="outline" aria-label={t("rangeLabel")} value={range} onValueChange={(v) => { if (v) onRange(v as HistoryRange); }}>
            {HISTORY_RANGES.map((r) => (
              <ToggleGroupItem key={r} value={r} className="px-2 text-xs">{t(`ranges.${RANGE_KEY[r]}`)}</ToggleGroupItem>
            ))}
          </ToggleGroup>
        </CardHeader>
        <CardContent className="flex flex-col gap-2 px-4">
          {body(() => <><StatusStrip buckets={h!.buckets} range={range} /><Axis buckets={h!.buckets} /><Legend /></>)}
        </CardContent>
      </Card>
      <Card className="min-w-0 gap-3 py-4">
        <CardHeader className="flex flex-row flex-wrap items-center gap-2 px-4">
          <CardTitle className="text-sm">{t("resources")}</CardTitle>
          <span className="flex-1" />
          <span className="text-xs text-muted-foreground">{t("sameRange")}</span>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 px-4">
          {body(() => <Resources history={h!} range={range} />)}
        </CardContent>
      </Card>
    </div>
  );

  function body(content: () => ReactNode) {
    if (query.isError && !h) return <QueryError compact error={query.error} onRetry={() => void query.refetch()} />;
    if (!h) return <Skeleton className="h-16 w-full" />;
    return content();
  }
}

function Uptime({ value }: { value: number | null }) {
  const t = useTranslations("history");
  if (value === null) return <span className="text-xs text-muted-foreground" data-uptime="none">{t("noData")}</span>;
  const tone = value > 99 ? "ok" : value > 95 ? "warn" : "err";
  return (
    <span className="text-sm font-semibold tabular text-(--tone)" data-tone={tone} data-uptime={tone} title={t("uptime")}>
      {formatNumber(value / 100, { style: "percent", maximumFractionDigits: 2 })}
    </span>
  );
}

function span(b: HistoryBucket, range: HistoryRange): string {
  return range === "24h" ? `${formatTime(b.start)}–${formatTime(b.end)}` : `${formatDateTime(b.start)} – ${formatDateTime(b.end)}`;
}

function StatusStrip({ buckets, range }: { buckets: HistoryBucket[]; range: HistoryRange }) {
  const t = useTranslations("history");
  return (
    <div role="list" aria-label={t("stripLabel")} className="group/strip flex h-8 items-stretch gap-px">
      {buckets.map((b) => (
        <Tooltip key={b.start}>
          <TooltipTrigger asChild>
            <span
              role="listitem"
              aria-label={`${span(b, range)}: ${t(scopedKey(HISTORY_STATE_KEYS[b.state], "history."))}`}
              data-state={b.state}
              data-tone={BUCKET_TONE[b.state] === "none" ? undefined : BUCKET_TONE[b.state]}
              className={cn(
                "min-w-0 flex-1 rounded-[2px] transition-[opacity,transform] duration-150 group-hover/strip:opacity-55 hover:scale-y-[1.12] hover:opacity-100!",
                b.state === "nodata" ? "border border-border bg-transparent" : b.state === "stopped" ? "bg-muted-foreground/55" : "bg-(--tone)",
                b.partial && "opacity-70",
              )}
            />
          </TooltipTrigger>
          <TooltipContent side="top" className="max-w-64">
            <BucketTip bucket={b} range={range} />
          </TooltipContent>
        </Tooltip>
      ))}
    </div>
  );
}

function BucketTip({ bucket: b, range }: { bucket: HistoryBucket; range: HistoryRange }) {
  const t = useTranslations("history");
  const lines = (["running", "unhealthy", "down", "stopped"] as const).filter((s) => b.minutes[s] > 0);
  return (
    <div className="flex flex-col gap-0.5" data-bucket-tip={b.state}>
      <b>{span(b, range)}</b>
      <span>{t(scopedKey(HISTORY_STATE_KEYS[b.state], "history."))}</span>
      {lines.map((s) => <span key={s}>{t("minutes", { state: t(scopedKey(HISTORY_STATE_KEYS[s], "history.")), minutes: Math.round(b.minutes[s]) })}</span>)}
      {b.minutes.stopped > 0 && <span className="opacity-80">{t("stoppedNote")}</span>}
      {b.exitCode !== undefined && b.minutes.down > 0 && <span>{t("exitCode", { code: b.exitCode })}</span>}
      {b.partial && <span className="opacity-80">{t("partial")}</span>}
    </div>
  );
}

function Axis({ buckets }: { buckets: HistoryBucket[] }) {
  const t = useTranslations("history");
  const first = buckets[0];
  const middle = buckets[Math.floor(buckets.length / 2)];
  if (!first || !middle) return null;
  return (
    <div className="flex justify-between text-[11px] text-muted-foreground tabular">
      <span>{formatDateTime(first.start)}</span>
      <span>{formatDateTime(middle.start)}</span>
      <span>{t("now")}</span>
    </div>
  );
}

function Legend() {
  const t = useTranslations("history");
  const states = ["running", "unhealthy", "down", "stopped", "nodata"] as const;
  return (
    <ul className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
      {states.map((s) => (
        <li key={s} className="flex items-center gap-1.5">
          <span
            aria-hidden
            data-tone={BUCKET_TONE[s] === "none" ? undefined : BUCKET_TONE[s]}
            className={cn("size-2.5 rounded-[2px]", s === "nodata" ? "border border-border" : s === "stopped" ? "bg-muted-foreground/55" : "bg-(--tone)")}
          />
          {t(scopedKey(HISTORY_STATE_KEYS[s], "history."))}
        </li>
      ))}
    </ul>
  );
}

function Resources({ history: h, range }: { history: RuntimeHistory; range: HistoryRange }) {
  const t = useTranslations("history");
  const none = t("noValues");
  const pct = (v?: number) => (v === undefined ? "–" : formatPercent(v));
  const bytes = (v?: number) => (v === undefined ? "–" : formatBytes(v));
  const cpuScale = Math.max(10, ...h.buckets.map((b) => b.cpuMax ?? 0));
  const memScale = h.memLimit ?? Math.max(1, ...h.buckets.map((b) => b.memMax ?? 0));
  return (
    <>
      <Metric
        label={t("cpu")}
        summary={h.cpuAvg === undefined ? none : t("cpuSummary", { avg: pct(h.cpuAvg), max: pct(h.cpuMax), now: pct(h.cpuNow) })}
        color="var(--chart-1)"
        buckets={h.buckets}
        range={range}
        value={(b) => b.cpuAvg}
        scale={cpuScale}
        tip={(b) => t("bucketCpu", { avg: pct(b.cpuAvg), max: pct(b.cpuMax) })}
        kind="cpu"
      />
      <Metric
        label={t("memory")}
        summary={h.memAvg === undefined ? none : h.memLimit ? t("memSummary", { avg: bytes(h.memAvg), max: bytes(h.memMax), limit: bytes(h.memLimit) }) : t("memSummaryNoLimit", { avg: bytes(h.memAvg), max: bytes(h.memMax) })}
        color="var(--chart-2)"
        buckets={h.buckets}
        range={range}
        value={(b) => b.memAvg}
        scale={memScale}
        tip={(b) => t("bucketMem", { avg: bytes(b.memAvg), max: bytes(b.memMax) })}
        kind="memory"
      />
      <dl className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {(["netRx", "netTx", "diskRead", "diskWrite"] as const).map((k) => (
          <div key={k} className="rounded-md border border-border px-2.5 py-1.5" data-total={k}>
            <dt className="text-[11px] text-muted-foreground">{t(`totals.${k}`)}</dt>
            <dd className="text-sm font-medium tabular">{formatBytes(h.totals[k])}</dd>
          </div>
        ))}
      </dl>
    </>
  );
}

function Metric({ label, summary, color, buckets, range, value, scale, tip, kind }: {
  label: string;
  summary: string;
  color: string;
  buckets: HistoryBucket[];
  range: HistoryRange;
  value: (b: HistoryBucket) => number | undefined;
  scale: number;
  tip: (b: HistoryBucket) => string;
  kind: string;
}) {
  const t = useTranslations("history");
  return (
    <div className="flex flex-col gap-1" data-metric={kind}>
      <div className="flex flex-wrap items-baseline gap-x-2 text-xs">
        <b className="font-medium">{label}</b>
        <span className="text-muted-foreground tabular">{summary}</span>
      </div>
      <div className="group/bars flex h-10 items-end gap-px" aria-hidden>
        {buckets.map((b) => {
          const v = value(b);
          return (
            <Tooltip key={b.start}>
              <TooltipTrigger asChild>
                <span className="flex h-full min-w-0 flex-1 items-end group-hover/bars:opacity-55 hover:opacity-100!">
                  <span className="w-full rounded-t-[2px]" style={{ height: v === undefined ? 0 : `${Math.max(4, Math.min(100, (v / scale) * 100))}%`, background: color }} />
                </span>
              </TooltipTrigger>
              <TooltipContent side="top">
                <b className="block">{span(b, range)}</b>
                {v === undefined ? t("noValues") : tip(b)}
              </TooltipContent>
            </Tooltip>
          );
        })}
      </div>
    </div>
  );
}
