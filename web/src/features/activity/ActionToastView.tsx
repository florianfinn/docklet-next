import { History, X } from "lucide-react";
import type { MouseEvent, ReactNode } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import type { ActionRun } from "@/domain/model";
import { cn } from "@/shared/lib/utils";
import type { ToastDesign, ToastDetail } from "@/shared/preferences/ui-style-store";
import { Button } from "@/shared/ui/button";

import { RunIconTile, StepTrack } from "./ActionSteps";
import { elapsedSeconds, RUN_ICON, RUN_TONE, runPhase, TOAST_LIFETIME } from "./run-phase";

export interface ActionToastViewProps {
  run: ActionRun;
  design: ToastDesign;
  detail: ToastDetail;
  now: number;
  /** Major actions open the centred dialog; others the action centre. */
  major: boolean;
  onClose: () => void;
  onOpen: () => void;
}

/** Controls inside the clickable toast act on their own and never open the details as well. */
const own = (event: MouseEvent) => event.stopPropagation();

/** Texts every design shows; each fits one line so a toast keeps its size from start to result. */
function useToastFacts(run: ActionRun, now: number) {
  const t = useTranslations("activity");
  const { index, step } = runPhase(run);
  const seconds = elapsedSeconds(run, now);
  const done = run.steps.filter((s) => s.state === "done").length;
  const status =
    run.status === "running" && step
      ? t("phase", { current: index + 1, total: run.steps.length, label: step.label })
      : run.status === "failed"
        ? t("failedAt", { label: step?.label ?? "" })
        : t("done", { seconds });
  const line = run.status === "failed" ? (run.error ?? t("toast.unknownError")) : run.status === "running" ? (step?.detail ?? run.target) : run.target;
  return { status, line, seconds, done, total: run.steps.length };
}

type Facts = ReturnType<typeof useToastFacts>;

function CloseButton({ onClose, className }: { onClose: () => void; className?: string }) {
  const t = useTranslations("activity");
  return (
    <button
      type="button"
      onClick={(e) => {
        own(e);
        onClose();
      }}
      className={cn("relative grid size-8 shrink-0 place-items-center rounded-md text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring", className)}
      aria-label={t("dismiss")}
    >
      <X className="size-4" />
    </button>
  );
}

/**
 * The title doubles as the keyboard entry to the details; a click anywhere
 * else on the toast does the same through the card's handler.
 */
function OpenTitle({ run, major, onOpen, className }: Pick<ActionToastViewProps, "run" | "major" | "onOpen"> & { className?: string }) {
  const t = useTranslations("activity.toast");
  return (
    <button
      type="button"
      data-slot="toast-open"
      onClick={(e) => {
        own(e);
        onOpen();
      }}
      aria-label={major ? t("openFlowFor", { title: run.title }) : t("openCentreFor", { title: run.title })}
      title={run.title}
      className={cn("block max-w-full truncate text-left font-semibold outline-none", className)}
    >
      {run.title}
    </button>
  );
}

/** Manual restore after a failure; the only extra action a toast keeps. */
function RestoreLink({ run, onClose }: { run: ActionRun; onClose: () => void }) {
  const t = useTranslations("activity");
  if (run.status !== "failed" || !run.restorePointId || !run.stackId) return null;
  return (
    <Button asChild size="sm" variant="outline" className="relative h-6 shrink-0 px-2 text-xs">
      <Link
        to={`/stacks/${run.stackId}?tab=deployments`}
        onClick={(e) => {
          own(e);
          onClose();
        }}
      >
        <History data-icon="inline-start" />
        {t("openRestore")}
      </Link>
    </Button>
  );
}

/** Countdown of a settled toast; the toast closes when the animation ends (paused on hover and focus by CSS). */
function LifeBar({ run, onEnd }: { run: ActionRun; onEnd: () => void }) {
  if (run.status === "running") return null;
  const life = TOAST_LIFETIME[run.status];
  return (
    <span aria-hidden className="absolute inset-x-0 bottom-0 h-1 bg-(--tone)/12" data-slot="toast-life-track">
      <span
        key={run.status}
        data-slot="toast-life"
        className="block h-full bg-(--tone)/70"
        style={{ "--life": `${life}ms`, "--life-steps": `steps(${Math.round(life / 1000)}, end)` } as React.CSSProperties}
        onAnimationEnd={(e) => e.target === e.currentTarget && onEnd()}
      />
    </span>
  );
}

function Meta({ run, facts }: { run: ActionRun; facts: Facts }) {
  const t = useTranslations("activity.toast");
  return (
    <dl className="flex h-4 min-w-0 items-center gap-x-4 overflow-hidden text-xs whitespace-nowrap text-muted-foreground" data-slot="toast-meta">
      <div className="flex min-w-16 gap-1">
        <dt className="sr-only">{t("target")}</dt>
        <dd className="truncate text-foreground/90">{run.target}</dd>
      </div>
      <div className="flex shrink-0 gap-1">
        <dt>{t("progress")}</dt>
        <dd className="tabular-nums">{t("stepsDone", { done: facts.done, total: facts.total })}</dd>
      </div>
      <div className="flex shrink-0 gap-1">
        <dt>{t("duration")}</dt>
        <dd className="tabular-nums">{t("seconds", { seconds: facts.seconds })}</dd>
      </div>
      {run.services.length > 0 && (
        <div className="flex min-w-0 gap-1">
          <dt>{t("services")}</dt>
          <dd className="truncate">{run.services.join(", ")}</dd>
        </div>
      )}
    </dl>
  );
}

function DetailRow({ run, facts, onClose, children }: { run: ActionRun; facts: Facts; onClose: () => void; children?: ReactNode }) {
  return (
    <div className="flex h-6 min-w-0 items-center gap-3">
      <p className={cn("min-w-0 flex-1 truncate text-xs leading-4", run.status === "failed" ? "font-medium text-destructive" : "text-muted-foreground")} data-slot="toast-detail" title={facts.line}>
        {facts.line}
      </p>
      {children}
      <RestoreLink run={run} onClose={onClose} />
    </div>
  );
}

type DesignProps = ActionToastViewProps & { facts: Facts };

/** "Leiste": glass card with title, key figures, the segment bar and one detail line. */
function BarDesign(props: DesignProps) {
  const { run, detail, facts, onClose } = props;
  const Icon = RUN_ICON[run.status];
  return (
    <div className="toast-card glass relative flex flex-col gap-3 overflow-hidden rounded-2xl border p-4 shadow-2xl">
      <span aria-hidden className="toast-edge absolute inset-x-0 top-0 h-0.5" />
      <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-3" data-slot="toast-header">
        <RunIconTile status={run.status} size="md">
          <Icon className={run.status === "running" ? "animate-spin" : "animate-pop-in"} />
        </RunIconTile>
        <div className="min-w-0">
          <OpenTitle {...props} className="text-[15px] leading-snug" />
          <div className="truncate text-xs leading-snug text-muted-foreground">{facts.status}</div>
        </div>
        <CloseButton onClose={onClose} className="-mt-1 -mr-1" />
      </div>
      {detail !== "compact" && <Meta run={run} facts={facts} />}
      <div data-slot="toast-track">
        <StepTrack steps={run.steps} status={run.status} labels={detail === "full"} />
      </div>
      <DetailRow run={run} facts={facts} onClose={onClose} />
      <LifeBar run={run} onEnd={onClose} />
    </div>
  );
}

/** "Kompakt": one row with status and percentage; the segment bar runs along the bottom edge. */
function CompactDesign(props: DesignProps) {
  const { run, facts, onClose } = props;
  const t = useTranslations("activity");
  const Icon = RUN_ICON[run.status];
  return (
    <div className="toast-card relative overflow-hidden rounded-xl border bg-popover/95 shadow-xl backdrop-blur">
      <div className="flex h-14 items-center gap-2.5 pr-1.5 pl-3" data-slot="toast-header">
        <span className="shrink-0 text-(--tone) [&_svg]:size-4.5">
          <Icon className={run.status === "running" ? "animate-spin" : "animate-pop-in"} aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <OpenTitle {...props} className="text-sm leading-tight" />
          <div className={cn("truncate text-xs leading-tight", run.status === "failed" ? "text-destructive" : "text-muted-foreground")} title={facts.line}>
            {run.status === "failed" ? facts.line : facts.status}
          </div>
        </div>
        <span className="w-10 shrink-0 text-right text-xs font-medium text-(--tone) tabular-nums">{t("toast.percent", { value: run.progress })}</span>
        <CloseButton onClose={onClose} />
      </div>
      <div className="px-3 pb-2" data-slot="toast-track">
        <StepTrack steps={run.steps} status={run.status} thin />
      </div>
      <LifeBar run={run} onEnd={onClose} />
    </div>
  );
}

/** "Statuskante": tinted surface with a status edge, kind label and named steps. */
function EdgeDesign(props: DesignProps) {
  const { run, detail, facts, onClose } = props;
  const t = useTranslations("activity");
  return (
    <div className="toast-card toast-card-edge tone-surface relative flex flex-col gap-2.5 overflow-hidden rounded-xl border border-l-4 bg-card/95 py-3 pr-3 pl-4 shadow-2xl backdrop-blur">
      <div className="flex items-start gap-2" data-slot="toast-header">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-[11px] font-semibold tracking-wide text-(--tone) uppercase">
            <span className={cn("size-1.5 shrink-0 rounded-full bg-(--tone)", run.status === "running" && "animate-breathe")} aria-hidden />
            <span className="truncate">
              {t(`runKind.${run.kind}`)} · {t(`status.${run.status}`)}
            </span>
          </div>
          <OpenTitle {...props} className="mt-0.5 text-[15px] leading-snug" />
          <div className="truncate text-xs leading-snug text-(--tone)">{facts.status}</div>
        </div>
        <span className="mt-0.5 shrink-0 text-xs text-muted-foreground tabular-nums">{t("toast.seconds", { seconds: facts.seconds })}</span>
        <CloseButton onClose={onClose} className="-mt-1" />
      </div>
      <div data-slot="toast-track">
        <StepTrack steps={run.steps} status={run.status} labels={detail !== "compact"} />
      </div>
      <DetailRow run={run} facts={facts} onClose={onClose} />
      <LifeBar run={run} onEnd={onClose} />
    </div>
  );
}

const DESIGNS = { bar: BarDesign, compact: CompactDesign, edge: EdgeDesign } as const;

/**
 * Progressive action toast. A click anywhere opens the run: major actions in
 * the action dialog, others in the action centre; the title is the keyboard
 * entry. Every design has a fixed set of single-line rows, so start, phase
 * changes, success and failure share one size.
 */
export function ActionToastView(props: ActionToastViewProps) {
  const { run, design, detail } = props;
  const facts = useToastFacts(run, props.now);
  const Design = DESIGNS[design];
  // Sonner gives unstyled toasts no width of their own; --width carries the design-lab width.
  return (
    <div
      data-tone={RUN_TONE[run.status]}
      data-slot="action-toast"
      data-design={design}
      data-status={run.status}
      data-detail={detail}
      data-major={props.major || undefined}
      role={run.status === "failed" ? "alert" : "status"}
      aria-live={run.status === "failed" ? "assertive" : "polite"}
      onClick={props.onOpen}
      className="action-toast w-(--width) max-w-full cursor-pointer text-sm"
    >
      <Design {...props} facts={facts} />
    </div>
  );
}
