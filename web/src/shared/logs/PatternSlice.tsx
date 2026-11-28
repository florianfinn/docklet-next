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

/** The lines of one pattern time bin: up to 2,000 lines / 1 MiB from its start, the pattern's lines marked. */
const SLICE_LINES = 2000;
/** Bin bounds in integer milliseconds, `end` exclusive (see `binBounds`). */
export interface SliceTarget { start: number; end: number }
// The browser keeps millisecond timestamps while Docker filters producer nanoseconds, so `until`
// covers every nanosecond of the bin's last millisecond.
const untilOf = (end: number) => new Date(end - 1).toISOString().replace("Z", "999999Z");
export function PatternSlice({ kind, id, pattern, slice, close }: { kind: "stack" | "container"; id: string; pattern: LogPattern; slice: SliceTarget; close: () => void }) {
  const t = useTranslations("stacks.logs.patterns"), adapter = useAdapter();
  const lifetime = logLifetime(adapter.subscribeLogs);
  const generation = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
  const key = JSON.stringify([kind, id, slice.start, slice.end, generation]);
  const [read, setRead] = useState<{ key: string; lines: LogLine[]; result?: LogReadResult }>();
  const list = useRef<HTMLOListElement>(null);
  useEffect(() => {
    const controller = new AbortController();
    const since = new Date(slice.start).toISOString(), until = untilOf(slice.end);
    void adapter.readLogs({ kind, id }, { since, until, tail: "all", maxLines: SLICE_LINES, maxBytes: 1024 * 1024 }, (lines) => {
      if (!controller.signal.aborted) setRead((old) => ({ key, lines: [...(old?.key === key ? old.lines : []), ...lines] }));
    }, controller.signal).then((result) => {
      if (!controller.signal.aborted) setRead((old) => ({ key, lines: old?.key === key ? old.lines : [], result }));
    }, () => {
      if (!controller.signal.aborted) setRead({ key, lines: [], result: { reason: "unavailable", lines: 0, bytes: 0 } });
    });
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- `key` carries kind, id, slice and generation.
  }, [adapter, key]);
  const current = read?.key === key ? read : undefined;
  const lines = useMemo(() => current?.lines ?? [], [current]);
  const members = useMemo(() => patternMembers(lines, pattern), [lines, pattern]);
  const marked = (line: LogLine) => members.has(line.id);
  const firstMatch = current?.result ? lines.find(marked)?.id : undefined;
  useEffect(() => {
    const box = list.current, row = firstMatch ? document.getElementById(`pattern-slice-${firstMatch}`) : null;
    // The list is positioned, so it is the rows' offset parent and offsetTop is already list-relative.
    if (box && row) box.scrollTop = row.offsetTop;
  }, [firstMatch]);
  const millis = slice.end - slice.start < 60_000, span = formatLogSpan(slice.start, slice.end - 1);
  return <div className="mt-2 grid gap-2 rounded-md border bg-muted/40 p-2" role="region" aria-label={t("sliceTitle", { span })}>
    <div className="flex flex-wrap items-center justify-between gap-2">
      <p className="text-xs font-medium">{t("sliceTitle", { span })}</p>
      <Button size="sm" variant="outline" onClick={close}>{t("sliceClose")}</Button>
    </div>
    {!current?.result && <p className="flex items-center gap-2 text-xs"><Spinner />{t("sliceReading")}</p>}
    {current?.result?.reason === "unavailable" && <p className="text-xs">{t("unavailable")}</p>}
    {(current?.result?.reason === "cap-lines" || current?.result?.reason === "cap-bytes") && <p className="text-xs">{t("sliceCapped", { count: lines.length })}</p>}
    {current?.result && current.result.reason !== "unavailable" && lines.length === 0 && <p className="text-xs">{t("sliceEmpty")}</p>}
    {lines.length > 0 && <ol ref={list} className="relative max-h-80 overflow-auto font-mono text-xs">{lines.map((line) => <li key={line.id} id={`pattern-slice-${line.id}`} data-match={marked(line) || undefined} className={`break-all px-1 ${marked(line) ? "bg-primary/15 text-foreground" : "text-muted-foreground"}`}>
      <time className="mr-2 text-primary">{formatLogStamp(Date.parse(line.ts), millis, false)}</time>{line.message}
    </li>)}</ol>}
  </div>;
}
