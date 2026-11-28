// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original client-side pattern folding, UI and regression evidence for M47.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { Button } from "@/shared/ui/button";
import { usePatternRead } from "./use-pattern-read";
import type { LogPattern } from "./patterns";

export function LogModeToggle({ mode, onChange }: { mode: "raw" | "patterns"; onChange: (mode: "raw" | "patterns") => void }) {
  const t = useTranslations("stacks.logs.patterns");
  return <SegmentedToggle value={mode} onChange={onChange} label={t("mode")} options={[{ value: "raw", label: t("raw") }, { value: "patterns", label: t("title") }]} fit="content" />;
}
export function PatternSamples({ pattern, clear }: { pattern: LogPattern; clear: () => void }) {
  const t = useTranslations("stacks.logs.patterns");
  return <div className="border-b p-3"><p>{t("fallback")}</p><pre className="overflow-auto whitespace-pre-wrap text-xs">{pattern.samples.join("\n")}</pre><Button variant="outline" size="sm" onClick={clear}>{t("clear")}</Button></div>;
}
export function PatternFilter({ clear }: { clear: () => void }) {
  const t = useTranslations("stacks.logs.patterns");
  return <Button variant="outline" size="sm" onClick={clear}>{t("clear")}</Button>;
}
export function PatternMode({ kind, id, select }: { kind: "stack" | "container"; id: string; select: (pattern: LogPattern) => void }) {
  const t = useTranslations("stacks.logs.patterns");
  const [window, setWindow] = useState("24");
  const [filter, setFilter] = useState("all");
  const [retry, setRetry] = useState(0);
  const view = usePatternRead(kind, id, Number(window), retry);
  const rows = view?.rows ?? [];
  const count = rows.filter((row) => !row.other).length;
  const visible = rows.filter((row) => filter === "all" || row.level === "error" || row.level === "warn").slice(0, 200);
  return <section className="space-y-3 p-3" aria-label={t("title")}>
    <div className="flex flex-wrap gap-2">
      <SegmentedToggle value={window} onChange={setWindow} label={t("window")} fit="content" options={(["1", "24", "168"] as const).map((value) => ({ value, label: t(`hours${value}`) }))} />
      <SegmentedToggle value={filter} onChange={setFilter} label={t("filter")} fit="content" options={[{ value: "all", label: t("all") }, { value: "warnings", label: t("warnings") }]} />
      <Button variant="outline" size="sm" onClick={() => setRetry((n) => n + 1)}>{t("retry")}</Button>
    </div>
    <p role="status">{t("progress", { lines: view?.lines ?? 0, bytes: view?.bytes ?? 0 })} {view?.first && view.last ? t("span", { first: view.first, last: view.last }) : t("noSpan")}</p>
    {!view?.result && <p>{t("reading")}</p>}
    {view?.result?.reason === "unavailable" && <p>{t("unavailable")}</p>}
    {(view?.result?.reason === "cap-lines" || view?.result?.reason === "cap-bytes") && <p>{t("capped")}</p>}
    {count > 2000 && <p>{t("variable", { count })}</p>}
    <p className="text-xs text-muted-foreground">{t("top")}</p>
    {visible.map((row) => <div key={row.other ? "other" : row.key} className="rounded border p-2 text-sm">
      <div className="flex flex-wrap gap-2"><span>{t(row.level)}</span><strong>×{row.count}</strong>
        {row.other ? <span>{t("other")}</span> : <button type="button" className="min-w-0 break-all text-left font-mono underline" onClick={() => select(row)}>{row.key}</button>}
      </div>
      <p className="text-xs text-muted-foreground">{t("span", { first: row.first, last: row.last })}</p>
      <div role="img" aria-label={t("distribution")} className="flex h-6 items-end gap-px">{row.bins.map((bin, i) => <span key={i} className="min-w-0 flex-1 bg-primary" style={{ height: `${bin / Math.max(1, ...row.bins) * 100}%` }} />)}</div>
      <details><summary>{t("samples")}</summary><pre className="overflow-auto whitespace-pre-wrap text-xs">{row.samples.join("\n")}</pre></details>
    </div>)}
  </section>;
}
