import { useDialogCommand, type DialogCommand } from "@/shared/components/use-dialog-command";
import { History, Layers, ListTree, X } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { useActionRun, useActionRuns } from "@/data/live";
import type { ActionRun } from "@/domain/model";
import { useStore } from "@/shared/lib/store";
import { cn } from "@/shared/lib/utils";
import { useUiStyle, type ActionDialogDesign } from "@/shared/preferences/ui-style-store";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/shared/ui/dialog";

import { actionDialogStore, closeActionDialog, openActionDialog } from "./action-dialog-store";
import { activityStore, openActivity } from "./activity-store";
import { RunBadge, RunIconTile, StepList, StepTrack } from "./ActionSteps";
import { useOutputOpen } from "./output-view-store";
import { elapsedSeconds, RUN_ICON, RUN_TONE, useNow } from "./run-phase";
import { RunErrorBox, RunFacts, RunFocusView, RunStatusLine } from "./RunFocus";
import { RuntimeControls } from "./RuntimeControls";
import { runtimeState } from "./runtime-presentation";
import { RunOutput } from "./RunOutput";

/** Whether a design opens the raw output by itself. */
const OUTPUT_OPEN: Record<ActionDialogDesign, boolean> = { focus: false, timeline: true, console: true };

/** "Zeitleiste": named segments, key figures, the step timeline beside them on wide screens, and the open raw output. */
function TimelineBody({ run, now }: { run: ActionRun; now: number }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <RunStatusLine run={run} className="shrink-0 text-sm" />
      <div className="shrink-0">
        <StepTrack steps={run.steps} status={run.status} labels />
      </div>
      <div className="grid shrink-0 gap-4 md:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)]">
        <StepList run={run} dense />
        <RunFacts run={run} now={now} className="content-start sm:grid-cols-2" />
      </div>
      <RunErrorBox run={run} />
      <RunOutput run={run} defaultOpen fill />
    </div>
  );
}

/** "Konsole": a compact progress strip on top, the raw output fills the rest. */
function ConsoleBody({ run, now }: { run: ActionRun; now: number }) {
  const t = useTranslations("activity.toast");
  const done = run.steps.filter((s) => s.state === "done").length;
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <div className="flex shrink-0 items-center gap-3">
        <RunStatusLine run={run} className="min-w-0 flex-1 text-sm" />
        <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
          {run.outcomeState === "unknown" ? "–" : t("stepsDone", { done, total: run.steps.length })} · {t("seconds", { seconds: elapsedSeconds(run, now) })}
        </span>
      </div>
      <div className="shrink-0">
        <StepTrack steps={run.steps} status={run.status} labels thin />
      </div>
      <RunErrorBox run={run} />
      <RunOutput run={run} defaultOpen fill />
    </div>
  );
}

function FocusBody({ run, now }: { run: ActionRun; now: number }) {
  return <RunFocusView run={run} now={now} fill />;
}

const BODIES: Record<ActionDialogDesign, typeof FocusBody> = { focus: FocusBody, timeline: TimelineBody, console: ConsoleBody };

function WaitingOffer({ waiting }: { waiting: string[] }) {
  const t = useTranslations("activity.dialog");
  const runs = useActionRuns();
  const offered = waiting.map((id) => runs.find((r) => r.id === id)).filter((r): r is ActionRun => !!r);
  const next = offered.at(-1);
  if (!next) return null;
  return (
    <div className="flex shrink-0 items-center gap-2 border-b bg-primary/8 px-4 py-2 text-xs" data-slot="dialog-waiting">
      <Layers className="size-3.5 shrink-0 text-primary-text" aria-hidden />
      <span className="min-w-0 flex-1 truncate">{t("waiting", { count: offered.length, title: next.title })}</span>
      <Button size="sm" variant="outline" className="h-7 text-xs" onClick={() => openActionDialog(next.id)}>
        {t("showWaiting")}
      </Button>
    </div>
  );
}

/**
 * Focus after closing. While the action centre is open, focus goes into it,
 * because any focus move outside closes that non-modal sheet. Otherwise an
 * auto-opened dialog may outlive the element that had focus; then focus goes
 * to the main content.
 */
function restoreFocus(event: Event, opener: Element | null): void {
  if (activityStore.get().open) {
    event.preventDefault();
    const sheet = document.querySelector<HTMLElement>('[data-layer="activity"]');
    const target = opener instanceof HTMLElement && opener.isConnected && sheet?.contains(opener) ? opener : sheet;
    requestAnimationFrame(() => target?.focus({ preventScroll: true }));
    return;
  }
  requestAnimationFrame(() => document.activeElement === document.body && document.getElementById("main-content")?.focus({ preventScroll: true }));
}

/**
 * Centred dialog for major actions. Its size follows the viewport and stays
 * the same through all phases; only opening the raw output grows it to the
 * full height, where the output is the one scroll area. It stays open after
 * the action settled until it is closed, and closing never stops the action.
 * A new major action is offered here instead of replacing the shown run.
 */
export function ActionDialog() {
  const { runId, waiting } = useStore(actionDialogStore, (s) => s);
  const run = useActionRun(runId);
  const feedback = useDialogCommand(!!run, run?.id);
  return (
    <Dialog open={!!run} onOpenChange={(open) => !open && feedback.change(false, closeActionDialog)}>
      {run && <ActionDialogContent run={run} waiting={waiting} feedback={feedback} />}
    </Dialog>
  );
}

function ActionDialogContent({ run, waiting, feedback }: { run: ActionRun; waiting: string[]; feedback: DialogCommand }) {
  const close = () => { feedback.dismiss(); closeActionDialog(); };
  const t = useTranslations("activity");
  const design = useUiStyle("actionDialogDesign");
  const now = useNow(run.status === "running");
  const expanded = useOutputOpen(run, OUTPUT_OPEN[design]);
  const Body = BODIES[design];
  const Icon = RUN_ICON[run.status];
  // Captured before the dialog takes focus.
  const [opener] = useState(() => document.activeElement);

  return (
    <DialogContent
      data-layer="action-dialog"
      data-design={design}
      data-status={run.status}
      data-tone={RUN_TONE[run.status]}
      data-expanded={expanded}
      showCloseButton={false}
      onCloseAutoFocus={(event) => restoreFocus(event, opener)}
      className={cn(
        "action-dialog flex max-h-[calc(100dvh-1rem)] flex-col gap-0 overflow-hidden p-0 sm:max-h-[calc(100dvh-2rem)] sm:max-w-[min(64rem,calc(100vw-4rem))] 2xl:max-w-[min(76rem,calc(100vw-4rem))]",
        expanded && "h-[calc(100dvh-1rem)] sm:h-[min(60rem,calc(100dvh-2rem))]",
      )}
    >
      <header className="flex shrink-0 items-start gap-3 border-b p-4 pr-3">
        <RunIconTile status={run.status} size="md">
          <Icon className={run.status === "running" ? "animate-spin" : "animate-pop-in"} />
        </RunIconTile>
        <div className="min-w-0 flex-1">
          <DialogTitle className="line-clamp-2 text-base leading-snug [overflow-wrap:anywhere]">{run.title}</DialogTitle>
          <DialogDescription className="truncate text-xs">
            {t(`runKind.${run.kind}`)} · {run.target}
          </DialogDescription>
        </div>
        <RunBadge status={run.status} className="mt-1 shrink-0">
          {runtimeState(run) ? t(`runtime.${runtimeState(run)!}`) : t(`status.${run.status}`)}
        </RunBadge>
        <Button size="icon" variant="ghost" className="size-8 shrink-0" onClick={close} aria-label={t("dialog.close")}>
          <X />
        </Button>
      </header>
      <WaitingOffer waiting={waiting} />
      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-4 sm:p-5" data-slot="dialog-body">
        <RuntimeControls key={`controls-${run.id}`} run={run} feedback={feedback} />
        <Body key={`body-${run.id}`} run={run.outcomeState === "unknown" ? { ...run, steps: [], progress: 0 } : run} now={now} />
      </div>
      <footer className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 border-t px-4 py-3">
        <p className="min-w-48 flex-1 text-[11px] text-muted-foreground">{t("dialog.closeHint")}</p>
        <div className="ml-auto flex flex-wrap items-center justify-end gap-2">
          {run.status === "failed" && run.restorePointId && run.stackId && (
            <Button asChild size="sm" variant="outline">
              <Link to={`/stacks/${run.stackId}?tab=deployments`} onClick={close}>
                <History data-icon="inline-start" />
                {t("openRestore")}
              </Link>
            </Button>
          )}
          <Button
            size="sm"
            variant="ghost"
            onClick={() => {
              close();
              openActivity(run.id);
            }}
          >
            <ListTree data-icon="inline-start" />
            {t("dialog.openCentre")}
          </Button>
          <Button size="sm" variant="secondary" onClick={close}>
            {t("dialog.closeShort")}
          </Button>
        </div>
      </footer>
    </DialogContent>
  );
}
