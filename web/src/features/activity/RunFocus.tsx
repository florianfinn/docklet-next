import { useTranslations } from "use-intl";

import { withPercent } from "@/domain/locale";
import type { ActionRun } from "@/domain/model";
import { cn } from "@/shared/lib/utils";

import { RunIconTile, StepTrack } from "./ActionSteps";
import { elapsedSeconds, RUN_ICON, RUN_TONE, runPhase } from "./run-phase";
import { RunOutput } from "./RunOutput";

export function RunStatusLine({ run, className }: { run: ActionRun; className?: string }) {
  const t = useTranslations("activity");
  const { index, step } = runPhase(run);
  const text = run.status === "running" && step ? t("phase", { current: index + 1, total: run.steps.length, label: step.label }) : run.status === "failed" ? t("failedAt", { label: step?.label ?? "" }) : t("dialog.result.succeeded");
  return (
    <p role="status" aria-live="polite" data-tone={RUN_TONE[run.status]} className={cn("truncate font-medium text-(--tone)", className)}>
      {text}
    </p>
  );
}

/** Key figures; target and services wrap instead of being cut off, so every affected service stays readable. */
export function RunFacts({ run, now, className }: { run: ActionRun; now: number; className?: string }) {
  const t = useTranslations("activity.dialog");
  const done = run.steps.filter((s) => s.state === "done").length;
  const items = [
    ["progress", t("progress"), `${done}/${run.steps.length} · ${withPercent(run.progress)}`],
    ["elapsed", t("elapsed"), `${elapsedSeconds(run, now)} s`],
    ["target", t("target"), run.target],
    ["services", t("services"), run.services.join(", ") || t("none")],
  ] as const;
  return (
    <dl className={cn("grid grid-cols-2 gap-x-4 gap-y-2 text-xs sm:grid-cols-[auto_auto_minmax(0,1fr)_minmax(0,2fr)]", className)} data-slot="dialog-facts">
      {items.map(([key, label, value]) => (
        <div key={key} className="min-w-0" data-fact={key}>
          <dt className="text-muted-foreground">{label}</dt>
          <dd className={cn("font-medium [overflow-wrap:anywhere]", (key === "progress" || key === "elapsed") && "whitespace-nowrap tabular-nums")}>{value}</dd>
        </div>
      ))}
    </dl>
  );
}

export function RunErrorBox({ run }: { run: ActionRun }) {
  const t = useTranslations("activity");
  if (run.status !== "failed") return null;
  const diagnosis = run.output.filter((l) => l.level === "error").slice(-2);
  return (
    <div className="shrink-0 rounded-xl border border-destructive/35 bg-destructive/8 p-3 text-xs" data-slot="dialog-error">
      <p className="font-medium text-destructive [overflow-wrap:anywhere]">{run.error ?? t("toast.unknownError")}</p>
      {diagnosis.length > 0 && <pre className="mt-2 rounded-lg bg-code p-2 font-mono text-[11px] leading-relaxed whitespace-pre-wrap text-destructive/90 [overflow-wrap:anywhere]">{diagnosis.map((l) => l.text).join("\n")}</pre>}
      {run.restorePointId && <p className="mt-1.5 text-muted-foreground">{t("restoreHint")}</p>}
    </div>
  );
}

/**
 * "Fokus": large status and phase, named segments, key figures and the error;
 * raw output below on demand. Used by the action dialog and, unchanged, by
 * the stack's Deployments tab. With `fill` the open output takes the remaining
 * height and is the only scroll area.
 */
export function RunFocusView({ run, now, fill = false, outputOpen = false, className }: { run: ActionRun; now: number; fill?: boolean; outputOpen?: boolean; className?: string }) {
  const t = useTranslations("activity");
  const { step } = runPhase(run);
  const Icon = RUN_ICON[run.status];
  return (
    <div className={cn("flex flex-col gap-4", fill && "min-h-0 flex-1", className)} data-slot="run-focus" data-run={run.id} data-status={run.status}>
      <div className="flex shrink-0 flex-col items-center gap-1.5 text-center">
        {/* In the dialog on phones the header icon suffices; the height goes to the output. */}
        <RunIconTile status={run.status} size="lg" className={cn("size-12 [&_svg]:size-6", fill && "max-sm:hidden")}>
          <Icon className={run.status === "running" ? "animate-spin" : "animate-pop-in"} />
        </RunIconTile>
        <RunStatusLine run={run} className="max-w-full text-base" />
        <p className="h-4 max-w-full truncate text-xs text-muted-foreground" title={step?.detail}>
          {run.status === "running" ? (step?.detail ?? run.target) : t(`dialog.result.${run.status}`)}
        </p>
      </div>
      <div className="shrink-0">
        <StepTrack steps={run.steps} status={run.status} labels />
      </div>
      <RunFacts run={run} now={now} className="shrink-0 rounded-xl border bg-background/40 p-3" />
      <RunErrorBox run={run} />
      <RunOutput run={run} defaultOpen={outputOpen} fill={fill} className={cn(fill && "min-h-0")} />
    </div>
  );
}
