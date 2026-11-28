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

/** The lines of one pattern time bin, read in pages of 500 lines / 1 MiB from its start, the pattern's lines marked. */
export const SLICE_PAGE = 500;
/** Bin bounds in integer milliseconds, `end` exclusive (see `binBounds`). */
export interface SliceTarget { start: number; end: number }
// The browser keeps millisecond timestamps while Docker filters producer nanoseconds, so `until`
// covers every nanosecond of the bin's last millisecond.
const untilOf = (end: number) => new Date(end - 1).toISOString().replace("Z", "999999Z");
const identity = (line: LogLine) => `${line.service}\u0001${line.ts}\u0001${line.message}`;
/** `from` is how many lines were shown before the current page; a page that adds none offers no further page. */
interface SliceRead { key: string; page: number; from: number; lines: LogLine[]; result?: LogReadResult }
/**
 * Bounds of the next page. It starts at the last shown millisecond again, because the browser cannot
 * name the producer nanosecond in between, and skips the lines of that millisecond it already shows.
 */
function nextPage(shown: readonly LogLine[], start: number) {
  const last = shown.at(-1);
  if (!last) return { since: start, skip: new Map<string, number>(), skipped: 0 };
  const since = Date.parse(last.ts), skip = new Map<string, number>();
  let skipped = 0;
  for (const line of shown) if (Date.parse(line.ts) === since) { skip.set(identity(line), (skip.get(identity(line)) ?? 0) + 1); skipped++; }
  return { since, skip, skipped };
}
export function PatternSlice({ kind, id, pattern, slice, close }: { kind: "stack" | "container"; id: string; pattern: LogPattern; slice: SliceTarget; close: () => void }) {
  const t = useTranslations("stacks.logs.patterns"), adapter = useAdapter();
  const lifetime = logLifetime(adapter.subscribeLogs);
  const generation = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
  const key = JSON.stringify([kind, id, slice.start, slice.end, generation]);
  const [read, setRead] = useState<SliceRead>();
  const current = read?.key === key ? read : undefined;
  const page = current?.page ?? 0;
  const cursor = useRef(nextPage([], slice.start));
  const list = useRef<HTMLOListElement>(null);
  useEffect(() => {
    const controller = new AbortController();
    const { since, skip, skipped } = page === 0 ? nextPage([], slice.start) : cursor.current;
    const append = (lines: LogLine[], result?: LogReadResult) => setRead((old) => ({ key, page, from: old?.key === key ? old.from : 0, lines: [...(old?.key === key ? old.lines : []), ...lines], result }));
    void adapter.readLogs({ kind, id }, { since: new Date(since).toISOString(), until: untilOf(slice.end), tail: "all", maxLines: SLICE_PAGE + skipped, maxBytes: 1024 * 1024 }, (lines) => {
      if (controller.signal.aborted) return;
      append(lines.filter((line) => {
        const left = Date.parse(line.ts) === since ? skip.get(identity(line)) : undefined;
        if (left) skip.set(identity(line), left - 1);
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
    cursor.current = nextPage(current.lines, slice.start);
    setRead({ ...current, page: current.page + 1, from: current.lines.length, result: undefined });
  };
  const lines = useMemo(() => current?.lines ?? [], [current]);
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
    {((capped && progressed) || (current?.result?.reason === "unavailable" && page > 0)) && <Button size="sm" variant="outline" className="justify-self-start" onClick={more}>{t("sliceLoadMore")}</Button>}
  </div>;
}
