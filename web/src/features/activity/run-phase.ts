import { CheckCircle2, Loader2, XCircle } from "lucide-react";
import { useEffect, useState } from "react";

import type { ActionRun, ActionStatus, ActionStep, StepState } from "@/domain/model";

/** Current (running or failed) step of a run. */
export function runPhase(run: ActionRun): { index: number; step: ActionStep | undefined } {
  const index = run.steps.findIndex((s) => s.state === "running" || s.state === "error");
  return { index, step: run.steps[index] };
}

/** How long a settled action toast stays; failures stay longer so the diagnosis can be read. */
export const TOAST_LIFETIME = { succeeded: 8000, failed: 20000 } as const;

/**
 * data-tone of run surfaces. "run" is the brand primary with the secondary
 * colour as edge (notifications.css); success and failure colours appear only
 * once a run has settled.
 */
export const RUN_TONE = { running: "run", succeeded: "ok", failed: "err" } as const;

export const RUN_ICON = { running: Loader2, succeeded: CheckCircle2, failed: XCircle } as const;

/** Step colours: brand while the run is running, semantic once it has settled. */
export function stepTone(state: StepState, status: ActionStatus): "run" | "ok" | "err" | "gray" {
  if (state === "error") return "err";
  if (state === "pending" || state === "skipped") return "gray";
  if (state === "running") return "run";
  return status === "running" ? "run" : "ok";
}

/** Whole seconds since the start, at least 1; settled runs use their end time. */
export function elapsedSeconds(run: Pick<ActionRun, "startedAt" | "finishedAt">, now: number): number {
  const end = run.finishedAt ? new Date(run.finishedAt).getTime() : now;
  return Math.max(1, Math.round((end - new Date(run.startedAt).getTime()) / 1000));
}

/** Clock for running actions, updated once per second; frozen while nothing runs. */
export function useNow(running: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!running) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [running]);
  return now;
}
