import { HISTORY_LIMITS, pruneRuns } from "@/domain/activity-history";
import type { ActionKind, ActionRun, ActionStep, DeployStepKey, RunOutputLine } from "@/domain/model";

import { genericScript, SIMULATED_NOTE, type OutputScript, type OutputSpec } from "./run-output";
import { simulationStore } from "./simulation";

export interface StepPlan {
  key: DeployStepKey;
  label: string;
  detail?: string;
  /** Base duration before the action-speed multiplier. */
  ms?: number;
}

export interface RunPlan {
  recreateResult?: ActionRun["recreateResult"];
  kind: ActionKind;
  title: string;
  target: string;
  hostId?: string;
  stackId?: string;
  containerId?: string;
  services?: string[];
  steps: StepPlan[];
  /** Step key at which the run fails; undefined runs to success. */
  failAt?: DeployStepKey;
  failMessage?: string;
  restorePointId?: string;
  /** Raw output per step; a generic one is used when absent. */
  output?: OutputScript;
  onStart?: (run: ActionRun) => void;
  onSuccess?: (run: ActionRun) => void;
  onFailure?: (run: ActionRun, step: StepPlan) => void;
}

type Listener = (runs: ActionRun[]) => void;

/** Drives simulated step progressions with timers; no Docker calls. */
export class ActionRunner {
  private runs: ActionRun[] = [];
  private readonly listeners = new Set<Listener>();
  private seq = 1;
  private readonly maxFinished: () => number;

  /** Seed runs get their output from `seedOutput`, stamped with their step times; `maxFinished` reads the hub's history limit. */
  constructor(
    seed: Omit<ActionRun, "output">[],
    seedOutput: (run: Omit<ActionRun, "output">) => OutputScript = () => genericScript,
    maxFinished: () => number = () => HISTORY_LIMITS.actions.default,
  ) {
    this.maxFinished = maxFinished;
    this.runs = pruneRuns(
      seed.map((r) => ({ ...structuredClone(r), output: replayOutput(r, seedOutput(r)) })),
      maxFinished(),
    );
  }

  /** Applies a changed history limit at once; running runs always stay. */
  prune(): void {
    const next = pruneRuns(this.runs, this.maxFinished());
    if (next.length === this.runs.length) return;
    this.runs = next;
    this.emit();
  }

  list(): ActionRun[] {
    return this.runs;
  }

  subscribe(listener: Listener): () => void {
    this.listeners.add(listener);
    listener(this.runs);
    return () => this.listeners.delete(listener);
  }

  start(plan: RunPlan): string {
    const id = `run-${Date.now().toString(36)}-${this.seq++}`;
    const now = new Date().toISOString();
    const run: ActionRun = {
      id,
      kind: plan.kind,
      title: plan.title,
      target: plan.target,
      hostId: plan.hostId,
      stackId: plan.stackId,
      containerId: plan.containerId,
      services: plan.services ?? [],
      status: "running",
      progress: 0,
      startedAt: now,
      restorePointId: plan.restorePointId,
      steps: plan.steps.map((s, i): ActionStep => ({
        key: s.key,
        label: s.label,
        detail: s.detail,
        state: i === 0 ? "running" : "pending",
        startedAt: i === 0 ? now : undefined,
      })),
      log: ["Gestartet (Simulation – keine Live-Aktion)"],
      output: [{ at: now, stream: "note", text: SIMULATED_NOTE }],
    };
    this.runs = pruneRuns([run, ...this.runs], this.maxFinished());
    plan.onStart?.(run);
    this.emit();
    this.advance(id, plan, 0);
    return id;
  }

  private advance(id: string, plan: RunPlan, index: number): void {
    const step = plan.steps[index];
    if (!step) return;
    const speed = Math.max(0.2, simulationStore.get().actionSpeed);
    const duration = (step.ms ?? 900 + Math.random() * 500) / speed;
    const script = plan.output ?? genericScript;
    const lines = script.lines(step);
    // Output lines arrive spread over the step, so a running action shows ongoing progress.
    lines.forEach((line, k) => setTimeout(() => this.append(id, step.key, [line]), (duration * (k + 1)) / (lines.length + 1)));
    setTimeout(() => {
      const failed = plan.failAt === step.key;
      const last = index === plan.steps.length - 1;
      const at = new Date().toISOString();
      // The patched run is taken before pruning, so callbacks do not depend on history retention.
      const settled = this.patch(id, (run) => {
        const steps = run.steps.map((s, i): ActionStep => {
          if (i === index) return { ...s, state: failed ? "error" : "done", finishedAt: at, detail: failed ? (plan.failMessage ?? s.detail) : s.detail };
          if (failed && i > index) return { ...s, state: "skipped" };
          if (!failed && i === index + 1) return { ...s, state: "running", startedAt: at };
          return s;
        });
        const progress = Math.round(((index + (failed ? 0 : 1)) / plan.steps.length) * 100);
        const log = [...run.log, failed ? `Fehler bei ${step.label}: ${plan.failMessage ?? "unbekannt"}` : `${step.label}${step.detail ? ` – ${step.detail}` : ""}`];
        if (failed) return { ...run, steps, progress, status: "failed", finishedAt: at, error: plan.failMessage, log };
        if (last) return { ...run, recreateResult: plan.recreateResult, steps, progress: 100, status: "succeeded", finishedAt: at, log: [...log, "Abgeschlossen"] };
        return { ...run, steps, progress, log };
      });
      if (!settled) return;
      let run = settled;
      if (failed) {
        const lines = script.failure(step, plan.failMessage ?? "unbekannter Fehler");
        run = { ...run, output: [...run.output, ...this.stamp(step.key, lines)] };
        this.runs = this.runs.map((r) => (r.id === id ? run : r));
        this.runs = pruneRuns(this.runs, this.maxFinished());
        this.emit();
      }
      if (failed) plan.onFailure?.(run, step);
      else if (last) plan.onSuccess?.(run);
      else this.advance(id, plan, index + 1);
    }, duration);
  }

  private stamp(step: DeployStepKey, lines: OutputSpec[]): RunOutputLine[] {
    const at = new Date().toISOString();
    return lines.map((l): RunOutputLine => ({ at, step, ...l }));
  }

  private append(id: string, step: DeployStepKey, lines: OutputSpec[]): void {
    if (!lines.length) return;
    this.patch(id, (run) => ({ ...run, output: [...run.output, ...this.stamp(step, lines)] }));
  }

  /** Applies `fn` to the run and returns the patched run, also when pruning drops it from the list. */
  private patch(id: string, fn: (run: ActionRun) => ActionRun): ActionRun | undefined {
    let patched: ActionRun | undefined;
    const before = this.runs.find((r) => r.id === id)?.status;
    this.runs = this.runs.map((r) => (r.id === id ? (patched = fn(r)) : r));
    // A run that just finished counts against the limit.
    if (before === "running" && patched && patched.status !== "running") this.runs = pruneRuns(this.runs, this.maxFinished());
    this.emit();
    return patched;
  }

  private emit(): void {
    this.listeners.forEach((l) => l(this.runs));
  }
}

/** Output of a finished seed run, timed by its recorded step times. */
function replayOutput(run: Omit<ActionRun, "output">, script: OutputScript): RunOutputLine[] {
  const lines: RunOutputLine[] = [{ at: run.startedAt, stream: "note", text: SIMULATED_NOTE }];
  for (const step of run.steps) {
    if (step.state === "pending" || step.state === "skipped") continue;
    const at = step.finishedAt ?? step.startedAt ?? run.startedAt;
    for (const l of script.lines(step)) lines.push({ at: step.startedAt ?? at, step: step.key, ...l });
    if (step.state === "error") for (const l of script.failure(step, run.error ?? step.detail ?? "unbekannter Fehler")) lines.push({ at, step: step.key, ...l });
  }
  return lines;
}
