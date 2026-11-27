// Source: docklet-next/issues/655
// Revision: 855c6a7f46fb9a7e2f4c3cc875b9f6e6d70e405d
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original per-service deploy stage list and behavioural coverage.

import { useTranslations } from "use-intl";

import { formatBytes } from "@/domain/format";
import type { ActionRun } from "@/domain/model";
import { cn } from "@/shared/lib/utils";
import { useUiStyle } from "@/shared/preferences/ui-style-store";

import { DEPLOY_STAGES, deployServicesOf, serviceSegments, serviceSummary, type DeployServiceRow, type ServiceSummary } from "./deploy-services";
import { stepTone } from "./run-phase";

const SUMMARY_TONE: Record<ServiceSummary["kind"], string> = {
  failed: "err", pulling: "run", building: "run", creating: "run", starting: "run", waiting: "run",
  healthy: "ok", ready: "ok", completed: "ok", running: "ok", interrupted: "gray", pending: "gray", notReached: "gray",
};

function ServiceLine({ row, run, prepared }: { row: DeployServiceRow; run: ActionRun; prepared: boolean }) {
  const t = useTranslations("activity.serviceStages");
  const motion = useUiStyle("runMotion");
  const segments = serviceSegments(row, run, prepared);
  const summary = serviceSummary(row, segments, run);
  const pull = row.pull;
  const text = summary.kind === "pulling"
    ? [t("summary.pulling", { percent: Math.floor(summary.fraction * 100) }), pull?.bytesTotal ? t("bytes", { done: formatBytes(pull.bytesDone!), total: formatBytes(pull.bytesTotal) }) : t("layers", { done: pull!.layersDone, total: pull!.layersTotal })].join(" · ")
    : t(`summary.${summary.kind}`);
  return (
    <li className="flex min-w-0 flex-col gap-1" data-service={row.name} data-summary={summary.kind}>
      <div className="flex min-w-0 items-baseline gap-2 text-xs">
        <span className="min-w-0 flex-1 truncate font-medium" title={row.name}>{row.name}</span>
        <span data-tone={SUMMARY_TONE[summary.kind]} className="shrink-0 text-(--tone) tabular-nums">{text}</span>
      </div>
      {/* The track is visual only; this names every stage and its state for assistive technology. */}
      <span className="sr-only" data-slot="service-stages-text">{segments.map((s) => t("stageState", { stage: t(`stage.${s.stage}`), state: t(`state.${s.state}`) })).join(", ")}</span>
      <ol className="grid grid-cols-5 gap-1" aria-hidden>
        {segments.map((s) => (
          <li
            key={s.stage}
            data-stage={s.stage}
            data-state={s.state}
            data-tone={s.state === "interrupted" ? "gray" : stepTone(s.state, run.status)}
            className={cn(
              "relative block h-1.5 overflow-hidden rounded-full bg-muted",
              (s.state === "done" || s.state === "error") && "bg-(--tone)",
              s.state === "running" && "bg-(--tone)/22",
              s.state === "skipped" && "border border-dashed border-muted-foreground/35 bg-transparent",
              s.state === "interrupted" && "bg-muted-foreground/20",
            )}
          >
            {s.state === "interrupted" && s.fraction !== undefined && <span data-slot="service-fill" className="absolute inset-y-0 left-0 bg-muted-foreground/55" style={{ width: `${Math.round(s.fraction * 100)}%` }} />}
            {s.state === "running" && (s.fraction === undefined
              ? <span data-slot="step-motion" data-variant={motion} className="absolute inset-0" />
              : <span data-slot="service-fill" className="absolute inset-y-0 left-0 bg-(--tone) transition-[width]" style={{ width: `${Math.round(s.fraction * 100)}%` }} />)}
          </li>
        ))}
      </ol>
    </li>
  );
}

/**
 * One row per Compose service with a five-segment track (download, build,
 * create, start, health). Renders nothing for runs without service rows, such
 * as older agents, so callers keep their aggregate view.
 */
export function ServiceProgress({ run, className }: { run: ActionRun; className?: string }) {
  const t = useTranslations("activity.serviceStages");
  const data = deployServicesOf(run);
  if (!data) return null;
  return (
    <section className={cn("flex shrink-0 flex-col gap-2 rounded-xl border bg-background/40 p-3", className)} data-slot="service-progress" aria-label={t("heading")}>
      <div className="grid grid-cols-5 gap-1 text-[10.5px] leading-tight text-muted-foreground" aria-hidden>
        {DEPLOY_STAGES.map((stage) => <span key={stage} className="truncate">{t(`stage.${stage}`)}</span>)}
      </div>
      {/* Long lists scroll on their own so an open raw output keeps its room. */}
      <ul className="-mr-1 flex max-h-[min(40dvh,22rem)] flex-col gap-2.5 overflow-y-auto pr-1" data-slot="service-list">
        {data.services.rows.map((row) => <ServiceLine key={row.name} row={row} run={run} prepared={data.prepared} />)}
      </ul>
      {data.services.truncated && <p className="text-[11px] text-muted-foreground">{t("truncated")}</p>}
    </section>
  );
}
