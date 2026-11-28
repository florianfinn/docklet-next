// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original client-side pattern folding, UI and regression evidence for M47.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { formatBytes } from "@/domain/format";
import { ChevronDown } from "lucide-react";
import { Button } from "@/shared/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from "@/shared/ui/dropdown-menu";
import { Input } from "@/shared/ui/input";
import { Spinner } from "@/shared/ui/spinner";
import { BinStrip } from "./BinStrip";
import { formatLogSpan } from "./log-time";
import { PatternSlice, type SliceTarget } from "./PatternSlice";
import { PATTERN_LIMITS, usePatternRead, type PatternRange } from "./use-pattern-read";
import { binBounds, type LogPattern } from "./patterns";

export function LogModeToggle({ mode, onChange }: { mode: "raw" | "patterns"; onChange: (mode: "raw" | "patterns") => void }) {
  const t = useTranslations("stacks.logs.patterns");
  return <SegmentedToggle value={mode} onChange={onChange} label={t("mode")} options={[{ value: "raw", label: t("raw") }, { value: "patterns", label: t("title") }]} fit="content" />;
}
export function PatternSamples({ pattern, clear }: { pattern: LogPattern; clear: () => void }) {
  const t = useTranslations("stacks.logs.patterns");
  return <div className="border-b p-3"><p>{t("fallback")}</p><pre className="overflow-auto overscroll-contain whitespace-pre text-xs">{pattern.samples.join("\n")}</pre><Button variant="outline" size="sm" onClick={clear}>{t("clear")}</Button></div>;
}
export function PatternFilter({ clear }: { clear: () => void }) {
  const t = useTranslations("stacks.logs.patterns");
  return <Button variant="outline" size="sm" onClick={clear}>{t("clear")}</Button>;
}
const PRESETS = ["1", "24", "168"] as const;
const rowId = (row: LogPattern) => (row.other ? "other" : `${row.level}\u0001${row.key}`);
/** `datetime-local` value in the browser's zone. */
const localInput = (ms: number) => { const d = new Date(ms - new Date(ms).getTimezoneOffset() * 60_000); return d.toISOString().slice(0, 16); };
export function PatternMode({ kind, id, select }: { kind: "stack" | "container"; id: string; select: (pattern: LogPattern) => void }) {
  const t = useTranslations("stacks.logs.patterns");
  const [window, setWindow] = useState("24");
  const [draft, setDraft] = useState(() => ({ from: localInput(Date.now() - 86_400_000), to: localInput(Date.now()) }));
  const [custom, setCustom] = useState<PatternRange>();
  const [filter, setFilter] = useState("all");
  const [limit, setLimit] = useState<number>(PATTERN_LIMITS.at(-1)!);
  // "Read again" refreshes only the shown window; other remembered windows keep their result.
  const [retries, setRetries] = useState<Record<string, number>>({});
  const [slice, setSlice] = useState<{ row: string } & SliceTarget>();
  const range: PatternRange = window === "custom" && custom ? custom : { hours: Number(window === "custom" ? "24" : window) };
  const shownWindow = JSON.stringify([range, limit]);
  const view = usePatternRead(kind, id, range, retries[shownWindow] ?? 0, limit);
  const rows = view?.rows ?? [];
  const count = rows.filter((row) => !row.other).length;
  const visible = rows.filter((row) => filter === "all" || row.level === "error" || row.level === "warn").slice(0, 200);
  const first = view?.first ? Date.parse(view.first) : NaN, last = view?.last ? Date.parse(view.last) : NaN;
  const draftSince = Date.parse(draft.from), draftUntil = Date.parse(draft.to);
  const draftValid = Number.isFinite(draftSince) && Number.isFinite(draftUntil) && draftSince < draftUntil;
  const choose = (value: string) => {
    setSlice(undefined); setWindow(value);
    if (value === "custom" && !custom && draftValid) setCustom({ since: new Date(draftSince).toISOString(), until: new Date(draftUntil).toISOString() });
  };
  const apply = () => { setSlice(undefined); setCustom({ since: new Date(draftSince).toISOString(), until: new Date(draftUntil).toISOString() }); };
  return <section className="space-y-3 p-3" aria-label={t("title")}>
    <div className="flex flex-wrap gap-2">
      <SegmentedToggle value={window} onChange={choose} label={t("window")} fit="content" options={[...PRESETS.map((value) => ({ value, label: t(`hours${value}`) })), { value: "custom", label: t("custom") }]} />
      <SegmentedToggle value={filter} onChange={setFilter} label={t("filter")} fit="content" options={[{ value: "all", label: t("all") }, { value: "warnings", label: t("warnings") }]} />
      <Button variant="outline" size="sm" onClick={() => { setSlice(undefined); setRetries((r) => ({ ...r, [shownWindow]: (r[shownWindow] ?? 0) + 1 })); }}>{t("retry")}</Button>
    </div>
    {window === "custom" && <div className="flex flex-wrap items-end gap-2">
      <label className="grid gap-1 text-xs">{t("from")}<Input type="datetime-local" value={draft.from} onChange={(e) => setDraft((d) => ({ ...d, from: e.target.value }))} /></label>
      <label className="grid gap-1 text-xs">{t("to")}<Input type="datetime-local" value={draft.to} onChange={(e) => setDraft((d) => ({ ...d, to: e.target.value }))} /></label>
      <Button size="sm" disabled={!draftValid} onClick={apply}>{t("apply")}</Button>
      {!draftValid && <p className="text-xs text-destructive">{t("invalidRange")}</p>}
    </div>}
    <div role="status" className="flex flex-wrap items-center gap-2 text-sm">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button type="button" aria-label={t("limitLabel", { lines: view?.lines ?? 0, limit })} className="inline-flex items-center gap-1 rounded-full bg-primary/15 px-2.5 py-0.5 font-medium text-primary">
            {t("lines", { lines: view?.lines ?? 0 })}<ChevronDown className="size-3.5" aria-hidden="true" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuLabel>{t("limit")}</DropdownMenuLabel>
          <DropdownMenuRadioGroup value={String(limit)} onValueChange={(value) => { setSlice(undefined); setLimit(Number(value)); }}>
            {PATTERN_LIMITS.map((value) => <DropdownMenuRadioItem key={value} value={String(value)}>{t("limitOption", { count: value })}</DropdownMenuRadioItem>)}
          </DropdownMenuRadioGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      <span className="rounded-full bg-info/15 px-2.5 py-0.5 font-medium text-info">{formatBytes(view?.bytes ?? 0)}</span>
      <span className="rounded-full bg-success/15 px-2.5 py-0.5 font-medium text-success">{Number.isFinite(first) && Number.isFinite(last) ? formatLogSpan(first, last) : t("noSpan")}</span>
    </div>
    {!view?.result && <p className="flex items-center gap-2 text-sm text-muted-foreground"><Spinner className="text-primary" />{t("reading")}</p>}
    {view?.result?.reason === "unavailable" && <p>{t("unavailable")}</p>}
    {(view?.result?.reason === "cap-lines" || view?.result?.reason === "cap-bytes") && <p>{t("capped")}</p>}
    {view?.result?.reason === "interrupted" && <p role="alert">{t("interrupted")}</p>}
    {count > 2000 && <p>{t("variable", { count })}</p>}
    <p className="text-xs text-muted-foreground">{t("top")}</p>
    {visible.map((row) => <div key={rowId(row)} className="rounded border p-2 text-sm">
      <div className="flex flex-wrap gap-2"><span>{t(row.level)}</span><strong>×{row.count}</strong>
        {row.other ? <span>{t("other")}</span> : <button type="button" className="min-w-0 break-all text-left font-mono underline" onClick={() => select(row)}>{row.key}</button>}
      </div>
      <BinStrip className="mt-6 h-8" label={t("distribution")} bins={row.bins.map((bin, i) => {
        const { start, end } = binBounds(i, first, last);
        const selected = slice?.row === rowId(row) && slice.start === start;
        return { tone: bin ? "bg-primary" : "bg-muted", height: bin ? bin / Math.max(1, ...row.bins) : 0, selected,
          label: t("binHint", { span: formatLogSpan(start, end - 1), count: bin }),
          onSelect: bin && Number.isFinite(first) ? () => setSlice(selected ? undefined : { row: rowId(row), start, end }) : undefined };
      })} />
      {slice?.row === rowId(row) && <PatternSlice kind={kind} id={id} pattern={row} slice={slice} close={() => setSlice(undefined)} />}
      <details><summary>{t("samples")}</summary><pre className="overflow-auto overscroll-contain whitespace-pre text-xs">{row.samples.join("\n")}</pre></details>
    </div>)}
  </section>;
}
