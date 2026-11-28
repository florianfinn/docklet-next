// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original bounded time-bin log read for the M47 pattern mode.

import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import type { LogReadResult } from "@/data/api";
import { logLifetime } from "@/data/log-buffer";
import type { LogLine } from "@/domain/model";
import { Button } from "@/shared/ui/button";
import { Spinner } from "@/shared/ui/spinner";
import { formatLogSpan, formatLogStamp } from "./log-time";
import { patternMembers, type LogPattern } from "./patterns";

/** The lines of one pattern time bin, read in pages of 500 lines / 1 MiB, the pattern's lines marked. */
export const SLICE_PAGE = 500;
const PAGE_BYTES = 1024 * 1024;
/** Bin bounds in integer milliseconds, `end` exclusive (see `binBounds`). */
export interface SliceTarget { start: number; end: number }
// The browser keeps millisecond timestamps while Docker filters producer nanoseconds, so `until`
// covers every nanosecond of the bin's last millisecond.
const untilOf = (end: number) => new Date(end - 1).toISOString().replace("Z", "999999Z");
const identity = (line: LogLine) => `${line.service}\u0001${line.ts}\u0001${line.message}`;
/**
 * A later page reads the bin from its start again: a stack merges its containers concurrently, so no
 * timestamp cursor is safe. Its budget is every shown line and byte plus one page, and the shown lines
 * are taken out as a multiset. Replayed lines can use at most the shown share of the budget, so in any
 * interleaving each page adds a full page of new lines (producer lines stay below 64 KiB) or ends the bin.
 */
interface Replay { seen: Map<string, number>; maxLines: number; maxBytes: number }
const firstPage = (): Replay => ({ seen: new Map(), maxLines: SLICE_PAGE, maxBytes: PAGE_BYTES });
const encoder = new TextEncoder();
/** `from` is how many lines were shown before this page. */
interface SliceRead { key: string; page: number; from: number; lines: LogLine[]; result?: LogReadResult }
export function PatternSlice({ kind, id, pattern, slice, close }: { kind: "stack" | "container"; id: string; pattern: LogPattern; slice: SliceTarget; close: () => void }) {
  const t = useTranslations("stacks.logs.patterns"), adapter = useAdapter();
  const lifetime = logLifetime(adapter.subscribeLogs);
  const generation = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
  const key = JSON.stringify([kind, id, slice.start, slice.end, generation]);
  const [read, setRead] = useState<SliceRead>();
  const current = read?.key === key ? read : undefined;
  const page = current?.page ?? 0;
  const replay = useRef(firstPage());
  const list = useRef<HTMLOListElement>(null);
  useEffect(() => {
    const controller = new AbortController();
    const { seen, maxLines, maxBytes } = page === 0 ? firstPage() : replay.current;
    const append = (lines: LogLine[], result?: LogReadResult) => setRead((old) => {
      const same = old?.key === key;
      return { key, page, from: same ? old.from : 0, lines: [...(same ? old.lines : []), ...lines], result };
    });
    void adapter.readLogs({ kind, id }, { since: new Date(slice.start).toISOString(), until: untilOf(slice.end), tail: "all", maxLines, maxBytes }, (lines) => {
      if (controller.signal.aborted) return;
      append(lines.filter((line) => {
        const left = seen.get(identity(line));
        if (left) seen.set(identity(line), left - 1);
        return !left;
      }));
    }, controller.signal).then((result) => {
      if (!controller.signal.aborted) append([], result);
    }, () => {
      if (!controller.signal.aborted) append([], { reason: "unavailable", lines: 0, bytes: 0 });
    });
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- `key` carries kind, id, slice and generation.
  }, [adapter, key, page]);
  const more = () => {
    if (!current) return;
    const seen = new Map<string, number>();
    let bytes = 0;
    for (const line of current.lines) { seen.set(identity(line), (seen.get(identity(line)) ?? 0) + 1); bytes += encoder.encode(line.message).length; }
    replay.current = { seen, maxLines: current.lines.length + SLICE_PAGE, maxBytes: bytes + PAGE_BYTES };
    setRead({ ...current, page: current.page + 1, from: current.lines.length, result: undefined });
  };
  // Pages arrive in producer order; a stable sort by time keeps each source's own order.
  const lines = useMemo(() => [...(current?.lines ?? [])].sort((x, y) => Date.parse(x.ts) - Date.parse(y.ts)), [current]);
  const members = useMemo(() => patternMembers(lines, pattern), [lines, pattern]);
  const firstMatch = current?.page === 0 && current.result ? members.indexOf(true) : -1;
  useEffect(() => {
    const box = list.current, row = firstMatch >= 0 ? box?.children[firstMatch] as HTMLElement | undefined : undefined;
    // The list is positioned, so it is the rows' offset parent and offsetTop is already list-relative.
    if (box && row) box.scrollTop = row.offsetTop;
  }, [firstMatch]);
  const millis = slice.end - slice.start < 60_000, span = formatLogSpan(slice.start, slice.end - 1);
  const capped = current?.result?.reason === "cap-lines" || current?.result?.reason === "cap-bytes";
  const progressed = lines.length > (current?.from ?? 0);
  return <div className="mt-2 grid min-w-0 gap-2 rounded-md border bg-muted/40 p-2" role="region" aria-label={t("sliceTitle", { span })}>
    <div className="flex flex-wrap items-center justify-between gap-2">
      <p className="text-xs font-medium">{t("sliceTitle", { span })}</p>
      <Button size="sm" variant="outline" onClick={close}>{t("sliceClose")}</Button>
    </div>
    {current?.result && current.result.reason !== "unavailable" && lines.length === 0 && <p className="text-xs">{t("sliceEmpty")}</p>}
    {lines.length > 0 && <ol ref={list} className="relative max-h-80 overflow-auto overscroll-contain font-mono text-xs">{lines.map((line, i) => <li key={i} data-match={members[i] || undefined} className={`w-max min-w-full whitespace-pre px-1 ${members[i] ? "bg-primary/15 text-foreground" : "text-muted-foreground"}`}>
      <time className="mr-2 text-primary">{formatLogStamp(Date.parse(line.ts), millis, false)}</time>{line.message}
    </li>)}</ol>}
    {!current?.result && <p className="flex items-center gap-2 text-xs"><Spinner />{t("sliceReading")}</p>}
    {current?.result?.reason === "unavailable" && <p className="text-xs">{t("unavailable")}</p>}
    {capped && <p className="text-xs">{t("sliceMore", { count: lines.length })}</p>}
    {current?.result?.reason === "interrupted" && <p role="alert" className="text-xs">{t("sliceInterrupted", { count: lines.length })}</p>}
    {((capped && progressed) || current?.result?.reason === "interrupted" || (current?.result?.reason === "unavailable" && page > 0)) && <Button size="sm" variant="outline" className="justify-self-start" onClick={more}>{t("sliceLoadMore")}</Button>}
  </div>;
}
