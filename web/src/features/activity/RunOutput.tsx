import { ChevronRight, Download, Terminal } from "lucide-react";
import { useEffect, useMemo, useRef } from "react";
import { useTranslations } from "use-intl";

import type { ActionRun } from "@/domain/model";
import { CopyButton } from "@/shared/components/code-block";
import { ToneBadge } from "@/shared/components/tone";
import { downloadBlob } from "@/shared/lib/tar";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/shared/ui/collapsible";

import { collapseOutput, type OutputRow } from "./output-collapse";
import { outputFallback, useOutputView } from "./output-view-store";
import { outputText, outputTime } from "./run-output-text";

function LineView({ row }: { row: OutputRow }) {
  const t = useTranslations("activity.output");
  const { line, frames } = row;
  return (
    <li
      data-stream={line.stream}
      data-level={line.level}
      data-frames={frames > 1 ? frames : undefined}
      title={frames > 1 ? t("folded", { count: frames }) : undefined}
      className={cn(
        "grid grid-cols-[6.6rem_3.6rem_minmax(0,1fr)] gap-x-2 border-l-2 border-transparent px-2 py-px",
        line.stream === "cmd" && "mt-1 text-primary-text",
        line.stream === "stderr" && "text-foreground/75",
        line.stream === "note" && "text-muted-foreground italic",
        line.level === "error" && "border-destructive bg-destructive/8 text-destructive",
        line.level === "warn" && "border-warning bg-warning/8 text-warning",
      )}
    >
      <time dateTime={line.at} className="text-muted-foreground tabular-nums">
        {outputTime(line.at)}
      </time>
      <span className="text-muted-foreground">{line.stream}</span>
      <span className="whitespace-pre-wrap [overflow-wrap:anywhere]">{line.stream === "cmd" ? `$ ${line.text}` : line.text}</span>
    </li>
  );
}

/**
 * Expandable raw Docker/Compose output of one action: chronological stdout
 * and stderr with timestamps, layer progress folded into one updating line,
 * live while the action runs, error diagnosis highlighted, copy and download.
 * View state belongs to the run, so every run keeps its own.
 */
export function RunOutput({ run, defaultOpen = false, fill = false, className }: { run: ActionRun; defaultOpen?: boolean; fill?: boolean; className?: string }) {
  const t = useTranslations("activity.output");
  const [view, setView] = useOutputView(run.id, outputFallback(run, defaultOpen));
  const listRef = useRef<HTMLOListElement>(null);
  const stick = useRef(true);
  // Folded rows keep their index, so it serves as a stable key.
  const rows = useMemo(() => collapseOutput(run.output).map((row, index) => ({ ...row, index })), [run.output]);
  const shown = view.onlyStderr ? rows.filter((r) => r.line.stream === "stderr") : rows;
  const errors = run.output.filter((l) => l.level === "error").length;
  const last = shown.at(-1);

  // Follows new and updated lines while the action runs, unless the reader scrolled up.
  useEffect(() => {
    const el = listRef.current;
    if (el && view.open && stick.current) el.scrollTop = el.scrollHeight;
  }, [view.open, shown.length, last?.line]);

  return (
    <Collapsible
      open={view.open}
      onOpenChange={(open) => setView({ open })}
      className={cn("rounded-xl border bg-background/40", fill && view.open && "flex min-h-0 flex-col", className)}
      data-slot="run-output"
      data-run={run.id}
    >
      <div className="flex flex-wrap items-center gap-2 px-3 py-2">
        <CollapsibleTrigger className="group/out flex min-w-0 items-center gap-1.5 rounded-md text-xs font-medium outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring" aria-label={view.open ? t("hide") : t("show")}>
          <ChevronRight className="size-3.5 transition-transform duration-(--dur-fast) group-data-[state=open]/out:rotate-90" aria-hidden />
          <Terminal className="size-3.5 text-muted-foreground" aria-hidden />
          {t("title")}
        </CollapsibleTrigger>
        <ToneBadge tone="violet" className="h-5 px-1.5 text-[10.5px]" title={`${t("hint")} ${t("stderrNote")} ${t("foldNote")}`}>
          {t("simulated")}
        </ToneBadge>
        <span className="text-[11px] text-muted-foreground tabular-nums">{t("lines", { count: rows.length })}</span>
        {errors > 0 && (
          <ToneBadge tone="err" className="h-5 px-1.5 text-[10.5px]">
            {t("errors", { count: errors })}
          </ToneBadge>
        )}
        {run.status === "running" && <span className="text-[11px] text-primary-text">{t("live")}</span>}
        {view.open && (
          <span className="ml-auto flex flex-wrap items-center gap-1.5">
            <Button type="button" size="sm" variant="ghost" className="h-7 text-xs" aria-pressed={view.onlyStderr} onClick={() => setView({ onlyStderr: !view.onlyStderr })}>
              {t("onlyErrors")}
            </Button>
            <CopyButton text={outputText(run)} label={t("copy")} className="h-7 text-xs" />
            <Button type="button" size="sm" variant="outline" className="h-7 text-xs" onClick={() => downloadBlob(`${run.id}.log`, outputText(run), "text/plain")}>
              <Download data-icon="inline-start" />
              {t("download")}
            </Button>
          </span>
        )}
      </div>
      <CollapsibleContent className={cn(fill && "flex min-h-0 flex-1 flex-col")}>
        {/* Filling a phone dialog, the hint stays on the badge's tooltip so the lines get the height. */}
        <p className={cn("px-3 pb-1.5 text-[11px] text-muted-foreground", fill && "max-sm:hidden")}>
          {t("hint")} {t("stderrNote")} {t("foldNote")}
        </p>
        {shown.length ? (
          <ol
            ref={listRef}
            tabIndex={0}
            aria-label={t("title")}
            onScroll={(e) => {
              const el = e.currentTarget;
              stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
            }}
            className={cn("overflow-auto border-t bg-code py-1.5 font-mono text-[11.5px] leading-relaxed outline-none focus-visible:ring-2 focus-visible:ring-ring", fill ? "min-h-0 flex-1" : "max-h-80")}
          >
            {shown.map((r) => (
              <LineView key={r.index} row={r} />
            ))}
          </ol>
        ) : (
          <p className="border-t px-3 py-2 text-xs text-muted-foreground">{t("empty")}</p>
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}
