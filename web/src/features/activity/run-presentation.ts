import { useEffect } from "react";

import type { ActionKind, ActionRun, ActionStatus } from "@/domain/model";
import { createStore } from "@/shared/lib/store";

import { runtimePresentation } from "./runtime-presentation";
import { DEPLOY_KINDS, isMajorAction } from "./action-policy";

/**
 * Where a run's progress is shown when it starts:
 * - `dialog`: the centred action dialog (major actions while automatic opening is on)
 * - `toast`: the progressive toast top right
 * - `inline`: a page surface that shows the run itself, e.g. the stack's Deployments tab
 */
export type ActionPresentation = "dialog" | "toast" | "inline";

/** Explicit request of a start site, matched to the next run of that stack with one of the kinds. */
export interface PresentationClaim {
  id: number;
  stackId: string;
  kinds: ReadonlySet<ActionKind>;
  presentation: ActionPresentation;
}

export interface PresentationState {
  claims: PresentationClaim[];
  /** Presentation bound to a run id, from a matched claim or once the start resolved. */
  byRun: Record<string, ActionPresentation>;
  /** Mounted inline surfaces per stack id. */
  surfaces: Record<string, number>;
}

const EMPTY: PresentationState = { claims: [], byRun: {}, surfaces: {} };

export const presentationStore = createStore<PresentationState>(EMPTY);

let claimSeq = 1;
/** Starts whose response is pending, and ids that left the run list meanwhile (not to be bound late). */
let pendingStarts = 0;
let presentationGeneration = 0;
export function resetActionIdentity(): void {
  ++presentationGeneration; pendingStarts = 0; retiredWhilePending.clear();
  presentationStore.set((state) => ({ claims: [], byRun: {}, surfaces: state.surfaces }));
}
const retiredWhilePending = new Set<string>();

/**
 * Presentation of a run that just started. A bound run id wins over a claim;
 * without either, major actions open the dialog when automatic opening is on.
 */
export function initialPresentation(run: Pick<ActionRun, "id" | "kind" | "stackId">, state: PresentationState, autoDialog: boolean): { presentation: ActionPresentation; claim?: PresentationClaim } {
  const bound = state.byRun[run.id];
  if (bound) return { presentation: bound };
  const claim = state.claims.find((c) => c.stackId === run.stackId && c.kinds.has(run.kind));
  if (claim) return { presentation: claim.presentation, claim };
  return { presentation: isMajorAction(run) && autoDialog ? "dialog" : "toast" };
}

/** True while a visible surface already shows the run, so its toast would duplicate it. */
export function isCovered(run: Pick<ActionRun, "id" | "kind" | "stackId">, dialogRunId: string | undefined, state: PresentationState): boolean {
  if (dialogRunId === run.id) return true;
  return state.byRun[run.id] === "inline" && !!run.stackId && (state.surfaces[run.stackId] ?? 0) > 0 && DEPLOY_KINDS.has(run.kind);
}

/** A current browser intent is identifiable without comparing server/device clocks. */
export function hasPresentationIntent(run: ActionRun, state: PresentationState): boolean {
  return runtimePresentation(run)?.currentIntent === true || state.byRun[run.id] !== undefined ||
    state.claims.some((claim) => claim.stackId === run.stackId && claim.kinds.has(run.kind));
}

export function bindPresentation(runId: string, presentation: ActionPresentation, consumed?: PresentationClaim): void {
  presentationStore.set((s) => ({
    ...s,
    claims: consumed ? s.claims.filter((c) => c.id !== consumed.id) : s.claims,
    byRun: s.byRun[runId] === presentation ? s.byRun : { ...s.byRun, [runId]: presentation },
  }));
}

/**
 * Starts an action with an explicit presentation. The claim is registered
 * before the start, because an adapter may report the run before its start
 * call resolves; the returned id is bound afterwards for adapters that report
 * it later.
 */
export async function startPresented<T extends string | undefined>(scope: { stackId: string; kinds?: ReadonlySet<ActionKind> }, presentation: ActionPresentation, start: () => Promise<T>): Promise<T> {
  const generation = presentationGeneration;
 const claim: PresentationClaim = { id: claimSeq++, stackId: scope.stackId, kinds: scope.kinds ?? DEPLOY_KINDS, presentation };
  presentationStore.set((s) => ({ ...s, claims: [...s.claims, claim] }));
  pendingStarts++;
  try {
    const runId = await start();
    if (generation !== presentationGeneration) return undefined as T;
    if (runId && !retiredWhilePending.has(runId)) bindPresentation(runId, presentation);
    return runId;
  } finally {
    if (generation === presentationGeneration && --pendingStarts === 0) retiredWhilePending.clear();
    presentationStore.set((s) => (s.claims.some((c) => c.id === claim.id) ? { ...s, claims: s.claims.filter((c) => c.id !== claim.id) } : s));
  }
}

/** Registers a page surface that shows the deploy runs of a stack while it is mounted. */
export function useInlineRunSurface(stackId: string): void {
  useEffect(() => {
    const change = (delta: number) =>
      presentationStore.set((s) => {
        const { [stackId]: _, ...rest } = s.surfaces;
        const count = Math.max(0, (s.surfaces[stackId] ?? 0) + delta);
        return { ...s, surfaces: count > 0 ? { ...rest, [stackId]: count } : rest };
      });
    change(1);
    return () => change(-1);
  }, [stackId]);
}

export interface RunToastPort {
  show(runId: string): void;
  hide(runId: string): void;
  offerDialog(runId: string): void;
}

export interface RunToastContext {
  autoDialog(): boolean;
  dialogRunId(): string | undefined;
  state(): PresentationState;
}

interface Seen {
  status: ActionStatus;
  covered: boolean;
}

/**
 * Keeps toasts in step with runs and the surfaces that already show them:
 * no toast while the dialog or an inline surface shows the run; when that
 * surface goes away, a still running run gets its toast back, a settled one
 * none. A failure that happens uncovered always shows, even after its toast
 * was closed. Runs present at the first sync are history and get no toast
 * of their own; they only follow later coverage changes. Later source histories
 * can seed their current status and coverage with the corresponding observation.
 */
export class RunToastCoordinator {
  private seen: Map<string, Seen> | null = null;
  private syncing = false;
  private again = false;
  private runs: ActionRun[] = [];
  private history = new Set<string>();
  private readonly port: RunToastPort;
  private readonly ctx: RunToastContext;

  constructor(port: RunToastPort, ctx: RunToastContext) {
    this.port = port;
    this.ctx = ctx;
  }

  /** History IDs seed silently for this observation; calls without runs only update presentation. */
  sync(runs?: ActionRun[], history?: ReadonlySet<string>): void {
    if (runs) { this.runs = runs; this.history = new Set(history); }
    // Offering the dialog or binding a claim notifies stores that call sync again.
    if (this.syncing) {
      this.again = true;
      return;
    }
    this.syncing = true;
    try {
      do {
        this.again = false;
        this.pass();
      } while (this.again);
    } finally {
      this.syncing = false;
    }
  }

  /** Brings back the toast of a run that a repeated start answered, unless a visible surface shows it. */
  reveal(runId: string): void {
    const run = this.runs.find((r) => r.id === runId);
    if (run && !this.covered(run)) this.port.show(runId);
  }

  private covered(run: ActionRun): boolean {
    return isCovered(run, this.ctx.dialogRunId(), this.ctx.state());
  }

  private pass(): void {
    if (!this.seen) {
      this.seen = new Map(this.runs.filter((r) => !hasPresentationIntent(r, this.ctx.state())).map((r) => [r.id, { status: r.status, covered: this.covered(r) }]));
      this.history.clear();
      this.visit();
      return;
    }
    // Only this observation seeds source history; reentrant presentation passes visit normally.
    const history = this.history;
    this.history = new Set();
    for (const run of this.runs) if (history.has(run.id)) this.seen.set(run.id, { status: run.status, covered: this.covered(run) });
    this.visit();
    this.forget();
  }

  /** Drops state of runs that left the list; they never come back. */
  private forget(): void {
    if (!this.seen) return;
    const live = new Set(this.runs.map((r) => r.id));
    const gone = [...this.seen.keys()].filter((id) => !live.has(id));
    if (!gone.length) return;
    for (const id of gone) {
      this.seen.delete(id);
      if (pendingStarts > 0) retiredWhilePending.add(id);
    }
    // Bindings of runs not yet reported stay: only ids that were seen are retired.
    if (gone.some((id) => id in presentationStore.get().byRun)) {
      presentationStore.set((s) => {
        const byRun = { ...s.byRun };
        for (const id of gone) delete byRun[id];
        return { ...s, byRun };
      });
    }
  }

  private visit(): void {
    if (!this.seen) return;
    // Oldest first, so several runs started at once keep their order in the dialog offer.
    for (const run of [...this.runs].reverse()) {
      const prev = this.seen.get(run.id);
      if (!prev) {
        if (run.status === "running" || run.status === "queued") {
          const { presentation, claim } = initialPresentation(run, this.ctx.state(), this.ctx.autoDialog());
          if (claim) bindPresentation(run.id, presentation, claim);
          if (presentation === "dialog") this.port.offerDialog(run.id);
        }
        const covered = this.covered(run);
        this.seen.set(run.id, { status: run.status, covered });
        if (!covered && (run.status === "running" || run.status === "queued" || run.status === "failed")) this.port.show(run.id);
        continue;
      }
      const covered = this.covered(run);
      if (covered && !prev.covered) this.port.hide(run.id);
      else if (!covered && prev.covered && (run.status === "running" || run.status === "queued")) this.port.show(run.id);
      else if (!covered && !prev.covered && run.status === "failed" && prev.status !== "failed") this.port.show(run.id);
      this.seen.set(run.id, { status: run.status, covered });
    }
  }
}
