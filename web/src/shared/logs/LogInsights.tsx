// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original M47 Health and Logs implementation and deterministic regression evidence.

import { useTranslations } from "use-intl";
import type { LogLine } from "@/domain/model";
import { Button } from "@/shared/ui/button";
import { densityBins } from "./derive";
import { BinStrip, type StripBin } from "./BinStrip";
import { formatLogStamp } from "./log-time";

export function RepeatButton({ count, expanded, onClick }: { count?: number; expanded: boolean; onClick: () => void }) {
  const t = useTranslations("stacks.logs");
  return count ? <button type="button" className="ml-2 rounded border px-1 font-sans" aria-expanded={expanded} aria-label={t(expanded ? "collapseRepeats" : "expandRepeats", { count })} onClick={onClick}>×{count}</button> : null;
}
type Translate = (key: "densityBin" | "densitySpan", values: Record<string, string | number>) => string;
function densityStrip(lines: readonly LogLine[], t: Translate, onSelect: (id: string) => void): StripBin[] {
  const bins = densityBins(lines);
  return bins.map((bin) => {
    const span = { start: formatLogStamp(bin.start), end: formatLogStamp(bin.end, false) };
    return bin.errors + bin.warnings > 0
      ? { tone: bin.errors ? "bg-destructive/70" : "bg-warning/70", label: t("densityBin", { errors: bin.errors, warnings: bin.warnings, ...span }), onSelect: () => bin.firstId && onSelect(bin.firstId) }
      : { tone: "bg-muted", label: t("densitySpan", span) };
  });
}
/** Reusable density over the loaded, filtered lines, independent of repeat presentation. */
export function LogDensity({ lines, byService = false, onSelect }: { lines: LogLine[]; byService?: boolean; onSelect: (id: string) => void }) {
  const t = useTranslations("stacks.logs");
  const sources = byService ? [...new Set(lines.map((l) => l.service))] : [""];
  return <div className="grid gap-1 border-b p-2">{sources.map((source) => <div key={source} className="flex items-center gap-2">
    {byService && <span className="w-28 truncate text-xs">{source}</span>}
    <BinStrip label={t("density")} bins={densityStrip(byService ? lines.filter((l) => l.service === source) : lines, (key, values) => t(key, values), onSelect)} />
  </div>)}</div>;
}
export function ErrorJump({ jump }: { jump: (direction: -1 | 1) => void }) {
  const t = useTranslations("stacks.logs");
  return <><Button size="sm" variant="outline" onClick={() => jump(-1)}>{t("previousError")}</Button><Button size="sm" variant="outline" onClick={() => jump(1)}>{t("nextError")}</Button></>;
}
