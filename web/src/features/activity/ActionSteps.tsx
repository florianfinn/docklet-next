import { Check, Circle, Loader2, Minus, X } from "lucide-react";
import type { ComponentProps, CSSProperties, ReactNode } from "react";
import { useTranslations } from "use-intl";

import type { ActionRun, ActionStatus, ActionStep, StepState } from "@/domain/model";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";
import { useUiStyle, type RunMotion } from "@/shared/preferences/ui-style-store";

import { stepTone } from "./run-phase";

/** Shared tone components know only semantic tones; a running run takes the brand primary. */
const RUNNING_STYLE = { "--tone": "var(--primary-text)" } as CSSProperties;
const SEMANTIC = { queued: "gray", running: "accent", succeeded: "ok", failed: "err", cancelled: "gray" } as const;

export function RunIconTile({ status, size, className, children }: { status: ActionStatus; size?: "sm" | "md" | "lg"; className?: string; children: ReactNode }) {
  return (
    <IconTile tone={SEMANTIC[status]} color={status === "running" ? "var(--primary-text)" : undefined} size={size} className={className}>
      {children}
    </IconTile>
  );
}

export function RunBadge({ status, style, ...props }: Omit<ComponentProps<typeof ToneBadge>, "tone"> & { status: ActionStatus }) {
  return <ToneBadge tone={SEMANTIC[status]} style={status === "running" ? { ...RUNNING_STYLE, ...style } : style} {...props} />;
}

function StepIcon({ state }: { state: StepState }) {
  if (state === "done") return <Check aria-hidden />;
  if (state === "error") return <X aria-hidden />;
  if (state === "running") return <Loader2 className="animate-spin" aria-hidden />;
  if (state === "skipped") return <Minus aria-hidden />;
  return <Circle className="opacity-40" aria-hidden />;
}

/**
 * Segmented progress bar, one segment per step. The running segment animates
 * in the motion chosen in the design lab (notifications.css); `motion`
 * overrides it for previews. Optional step
 * names sit on one truncated line under their segment, so the height never
 * changes between phases.
 */
export function StepTrack({ steps, status, labels = false, thin = false, motion }: { steps: ActionStep[]; status: ActionStatus; labels?: boolean; thin?: boolean; motion?: RunMotion }) {
  const chosen = useUiStyle("runMotion");
  return (
    <ol className="flex gap-1" aria-hidden data-slot="step-track" data-thin={thin || undefined}>
      {steps.map((s, i) => (
        <li key={s.key + i} data-tone={stepTone(s.state, status)} data-state={s.state} className="flex min-w-0 flex-1 flex-col gap-1">
          <span data-slot="step-segment" className={cn("relative block overflow-hidden rounded-full bg-muted", thin ? "h-1.5" : "h-2", (s.state === "done" || s.state === "error") && "bg-(--tone)", s.state === "running" && "bg-(--tone)/22")}>
            {s.state === "running" && <span data-slot="step-motion" data-variant={motion ?? chosen} className="absolute inset-0" />}
          </span>
          {labels && <span className={cn("truncate text-[10.5px] leading-tight", s.state === "running" || s.state === "error" ? "font-medium text-(--tone)" : "text-muted-foreground")}>{s.label}</span>}
        </li>
      ))}
    </ol>
  );
}

/** Vertical step list with state, label and detail (validate → health). */
export function StepList({ run, dense }: { run: ActionRun; dense?: boolean }) {
  const t = useTranslations("activity.step");
  return (
    <ol className={cn("relative flex flex-col", dense ? "gap-1.5" : "gap-2.5")}>
      {run.steps.map((s, i) => (
        <li key={s.key + i} className="relative flex gap-2.5">
          {i < run.steps.length - 1 && <span aria-hidden className="absolute top-6 left-[11px] h-[calc(100%-12px)] w-px bg-border" />}
          <span
            data-tone={stepTone(s.state, run.status)}
            className={cn(
              "z-10 grid size-6 shrink-0 place-items-center rounded-full border border-(--tone)/40 bg-card text-(--tone) [&_svg]:size-3.5",
              (s.state === "done" || s.state === "error") && "bg-(--tone)/15",
              s.state === "running" && "animate-pulse-ring",
            )}
          >
            <StepIcon state={s.state} />
          </span>
          <div className="min-w-0 pt-0.5">
            <div className={cn("text-sm leading-tight", s.state === "pending" || s.state === "skipped" ? "text-muted-foreground" : "font-medium")}>
              {s.label}
              <span className="sr-only"> – {t(s.state)}</span>
            </div>
            {s.detail && !dense && <div className={cn("text-xs [overflow-wrap:anywhere]", s.state === "error" ? "text-destructive" : "text-muted-foreground")}>{s.detail}</div>}
          </div>
        </li>
      ))}
    </ol>
  );
}
