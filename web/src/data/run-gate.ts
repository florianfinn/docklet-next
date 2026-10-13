import type { ActionRun } from "@/domain/model";

import type { DockletAdapter } from "./adapter";

/**
 * A repeat of a check that succeeded within this window returns that result.
 * Covers double clicks and several entry points; `retry` rechecks at once.
 * Failed checks and mutations have no freshness.
 */
export const CHECK_FRESH_MS = 30_000;

/** Why a start returned an existing run or was refused. */
export type GateReason = "inflight" | "fresh" | "busy";

export interface GateNotice {
  reason: GateReason;
  key: string;
  /** Run that answers the start (inflight, fresh) or that blocks it (busy). */
  runId?: string;
  title?: string;
  finishedAt?: string;
  /** Fresh results only: starts the same check once more, bypassing freshness. */
  retry?: () => Promise<unknown>;
}

/** A different mutation on the same target is still running; nothing was started. */
export class RunBusyError extends Error {
  readonly code = "busy";
  readonly runId: string | undefined;
  constructor(runId: string | undefined) {
    super("busy");
    this.name = "RunBusyError";
    this.runId = runId;
  }
}

export const isRunBusy = (e: unknown): e is RunBusyError => e instanceof RunBusyError;

/**
 * Same `key`: same operation and target, answered by the in-flight or fresh
 * run. Same `scope` with another key: a conflicting mutation, refused while
 * running. `coveredBy`: keys whose running start includes this one.
 */
export interface GateRule {
  key: string;
  scope?: string;
  freshMs?: number;
  coveredBy?: string[];
}

type Args = readonly unknown[];
const str = (v: unknown) => (typeof v === "string" ? v : "");
const list = (v: unknown) => (Array.isArray(v) ? v.map(String).toSorted().join(",") : "all");

/** Small stable hash for inputs that make two deploys different operations. */
export function contentHash(text: string): string {
  let h = 2166136261;
  for (let i = 0; i < text.length; i++) h = Math.imul(h ^ text.charCodeAt(i), 16777619);
  return (h >>> 0).toString(36);
}

/** Gated adapter methods: every start of a run, from any page, passes here. */
export const GATE_RULES: Partial<Record<keyof DockletAdapter, (args: Args) => GateRule>> = {
  checkUpdates: ([scope]) => {
    const s = (scope ?? {}) as { stackId?: string; containerId?: string };
    const key = s.stackId ? `check:stack:${s.stackId}` : s.containerId ? `check:container:${s.containerId}` : "check:all";
    return { key, freshMs: CHECK_FRESH_MS, coveredBy: key === "check:all" ? [] : ["check:all"] };
  },
  runScan: () => ({ key: "scan:all", freshMs: CHECK_FRESH_MS }),
  reconnectHost: ([hostId]) => ({ key: `reconnect:${str(hostId)}`, freshMs: CHECK_FRESH_MS }),
  runStackCommand: ([stackId, command]) => ({ key: `stack:${str(stackId)}:${str(command)}`, scope: `stack:${str(stackId)}` }),
  installStackUpdates: ([stackId, services]) => ({ key: `stack:${str(stackId)}:update:${list(services)}`, scope: `stack:${str(stackId)}` }),
  restoreRestorePoint: ([stackId, point]) => ({ key: `stack:${str(stackId)}:restore:${str(point)}`, scope: `stack:${str(stackId)}` }),
  restartService: ([stackId, service]) => ({ key: `stack:${str(stackId)}:restart:${str(service)}`, scope: `stack:${str(stackId)}` }),
  deployStack: ([input]) => {
    const d = (input ?? {}) as { stackId?: string | null; hostId?: string; name?: string; compose?: string; env?: string; labels?: string[] };
    const target = d.stackId ?? `new:${str(d.hostId)}:${str(d.name)}`;
    return { key: `deploy:${target}:${contentHash(JSON.stringify([d.compose, d.env, d.labels]))}`, scope: `stack:${target}` };
  },
  runContainerCommand: ([id, command]) => ({ key: `container:${str(id)}:${str(command)}`, scope: `container:${str(id)}` }),
  recreateContainer: ([id, input]) => ({ key: `container:${str(id)}:recreate:${contentHash(JSON.stringify(input))}`, scope: `container:${str(id)}` }),
  installContainerUpdate: ([id]) => ({ key: `container:${str(id)}:update`, scope: `container:${str(id)}` }),
  restartAgent: ([hostId]) => ({ key: `host:${str(hostId)}:agent-restart`, scope: `host:${str(hostId)}` }),
  connectHost: ([hostId]) => ({ key: `host:${str(hostId)}:connect`, scope: `host:${str(hostId)}` }),
  runBackup: ([volumeId]) => ({ key: `volume:${str(volumeId)}:backup`, scope: `volume:${str(volumeId)}` }),
  restoreBackup: ([volumeId, runId, mode]) => ({ key: `volume:${str(volumeId)}:restore:${str(runId)}:${str(mode)}`, scope: `volume:${str(volumeId)}` }),
  pullImage: ([input]) => {
    const p = (input ?? {}) as { hostId?: string; reference?: string };
    return { key: `image:${str(p.hostId)}:pull:${str(p.reference)}` };
  },
  pruneImages: ([input]) => {
    const p = (input ?? {}) as { hostIds?: string[]; mode?: string };
    return { key: `images:prune:${list(p.hostIds)}:${str(p.mode)}`, scope: "images:prune" };
  },
};

/** Run id inside a start's result: a plain id or a deploy's `{ actionId }`. */
function runIdOf(value: unknown): string | undefined {
  if (typeof value === "string") return value;
  if (value && typeof value === "object" && typeof (value as { actionId?: unknown }).actionId === "string") return (value as { actionId: string }).actionId;
  return undefined;
}

interface Entry {
  rule: GateRule;
  /** Stack scope of a container target; a mutation of that stack conflicts with it. */
  parent?: string;
  promise: Promise<unknown>;
  settled: boolean;
  settledAt?: number;
  value?: unknown;
  runId?: string;
}

/** A run id that never shows up in the run list stops blocking after this. */
const LOST_RUN_MS = 10_000;

type Status = ActionRun["status"] | "pending";
const active = (s: Status) => s === "pending" || s === "running";

/** Target scopes of a run, also for runs started outside the gate (schedules, other tabs). */
function scopesOf(run: ActionRun): string[] {
  if (run.kind === "check-updates" || run.kind === "scan") return [];
  return [run.stackId && `stack:${run.stackId}`, run.containerId && `container:${run.containerId}`].filter((x): x is string => !!x);
}

/** Resolves a container scope to its stack scope; the provider reads it from cached container data. */
export type ParentResolver = (scope: string) => string | undefined;

/**
 * Central de-duplication of action starts: identical starts share one run,
 * a fresh check answers repeats, a conflicting mutation is refused with
 * RunBusyError, unrelated targets never wait. Answers that started nothing
 * are reported to `subscribe` listeners.
 */
export class RunGate {
  private readonly entries = new Map<string, Entry>();
  private readonly runs = new Map<string, ActionRun>();
  private readonly listeners = new Set<(notice: GateNotice) => void>();
  private unwatch: (() => void) | undefined;
  private parentOf: ParentResolver = () => undefined;
  private readonly inner: DockletAdapter;
  private readonly now: () => number;

  constructor(inner: DockletAdapter, now: () => number = Date.now) {
    this.inner = inner;
    this.now = now;
  }

  subscribe(listener: (notice: GateNotice) => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  /** Sets the container-to-stack lookup; returns a cleanup that removes it. */
  setParentResolver(resolve: ParentResolver): () => void {
    this.parentOf = resolve;
    return () => {
      if (this.parentOf === resolve) this.parentOf = () => undefined;
    };
  }

  /** Number of remembered starts; settled ones leave once neither running nor fresh. */
  get size(): number {
    return this.entries.size;
  }

  /** The run list is followed only while starts are remembered. */
  private watch(): void {
    this.unwatch ??= this.inner.subscribeActions((runs) => {
      this.runs.clear();
      for (const r of runs) this.runs.set(r.id, r);
      this.prune();
    });
  }

  private prune(): void {
    for (const [key, e] of this.entries) {
      const st = this.status(e);
      if (active(st) || (st === "succeeded" && this.freshAt(e) !== undefined)) continue;
      this.entries.delete(key);
    }
    if (this.entries.size === 0 && this.unwatch) {
      const stop = this.unwatch;
      this.unwatch = undefined;
      this.runs.clear();
      stop();
    }
  }

  private status(entry: Entry): Status {
    if (!entry.settled) return "pending";
    const run = entry.runId ? this.runs.get(entry.runId) : undefined;
    if (run) return run.status;
    return entry.runId && this.now() - (entry.settledAt ?? 0) < LOST_RUN_MS ? "running" : "succeeded";
  }

  /** Finish time of a succeeded run while it is still fresh. */
  private freshAt(entry: Entry): number | undefined {
    const run = entry.runId ? this.runs.get(entry.runId) : undefined;
    const finished = run?.finishedAt ? Date.parse(run.finishedAt) : NaN;
    return entry.rule.freshMs && Number.isFinite(finished) && this.now() - finished < entry.rule.freshMs ? finished : undefined;
  }

  private notify(notice: GateNotice): void {
    this.listeners.forEach((l) => l(notice));
  }

  private answer(entry: Entry, reason: GateReason, retry?: () => Promise<unknown>): Promise<unknown> {
    const run = entry.runId ? this.runs.get(entry.runId) : undefined;
    this.notify({ reason, key: entry.rule.key, runId: entry.runId, title: run?.title, finishedAt: run?.finishedAt, retry });
    return entry.settled ? Promise.resolve(entry.value) : entry.promise;
  }

  /** Two scoped targets conflict when equal or when one is the other's stack. */
  private conflicts(a: { scope: string; parent?: string }, b: { scope: string; parent?: string }): boolean {
    return a.scope === b.scope || a.parent === b.scope || b.parent === a.scope;
  }

  private blockerOf(rule: GateRule, parent: string | undefined): string | undefined | false {
    const me = { scope: rule.scope!, parent };
    for (const e of this.entries.values()) {
      if (e.rule.key === rule.key || !e.rule.scope || !active(this.status(e))) continue;
      if (this.conflicts(me, { scope: e.rule.scope, parent: e.parent })) return e.runId;
    }
    const ownRuns = new Set([...this.entries.values()].filter((e) => e.rule.key === rule.key).map((e) => e.runId));
    for (const r of this.runs.values()) {
      if (r.status !== "running" || ownRuns.has(r.id)) continue;
      if (scopesOf(r).some((scope) => this.conflicts(me, { scope, parent: this.parentOf(scope) }))) return r.id;
    }
    return false;
  }

  /** Starts through the gate; `fresh: false` skips the freshness answer (explicit recheck). */
  run(method: keyof DockletAdapter, args: Args, options: { fresh?: boolean } = {}): Promise<unknown> {
    const ruleOf = GATE_RULES[method];
    const start = () => (this.inner[method] as (...a: Args) => Promise<unknown>).apply(this.inner, args as unknown[]);
    if (!ruleOf) return start();
    this.prune();
    this.watch();
    const rule = ruleOf(args);

    const same = this.entries.get(rule.key);
    if (same) {
      const st = this.status(same);
      if (active(st)) return this.answer(same, "inflight");
      if (options.fresh !== false && st === "succeeded" && this.freshAt(same) !== undefined) {
        return this.answer(same, "fresh", () => this.run(method, args, { fresh: false }));
      }
    }
    for (const cover of rule.coveredBy ?? []) {
      const other = this.entries.get(cover);
      if (other && active(this.status(other))) return this.answer(other, "inflight");
    }
    const parent = rule.scope ? this.parentOf(rule.scope) : undefined;
    if (rule.scope) {
      const blocker = this.blockerOf(rule, parent);
      if (blocker !== false) {
        this.notify({ reason: "busy", key: rule.key, runId: blocker, title: blocker ? this.runs.get(blocker)?.title : undefined });
        return Promise.reject(new RunBusyError(blocker));
      }
    }

    // Registered before the start: a run list update during the start must not prune it.
    const entry: Entry = { rule, parent, settled: false, promise: Promise.resolve() };
    this.entries.set(rule.key, entry);
    entry.promise = new Promise<unknown>((resolve) => resolve(start())).then(
      (value) => {
        entry.settled = true;
        entry.settledAt = this.now();
        entry.value = value;
        entry.runId = runIdOf(value);
        return value;
      },
      (error: unknown) => {
        if (this.entries.get(rule.key) === entry) this.entries.delete(rule.key);
        this.prune();
        throw error;
      },
    );
    return entry.promise;
  }
}

const gateOfProxy = new WeakMap<object, RunGate>();
const proxyOfInner = new WeakMap<DockletAdapter, DockletAdapter>();

/** The adapter behind its run gate; one gate per inner adapter, so remounts share it. */
export function withRunGate(inner: DockletAdapter, now?: () => number): DockletAdapter {
  if (gateOfProxy.has(inner)) return inner;
  const known = proxyOfInner.get(inner);
  if (known) return known;
  const gate = new RunGate(inner, now);
  const proxy = new Proxy(inner, {
    get(target, prop, receiver) {
      if (typeof prop === "string" && prop in GATE_RULES) return (...args: unknown[]) => gate.run(prop as keyof DockletAdapter, args);
      return Reflect.get(target, prop, receiver) as unknown;
    },
  });
  gateOfProxy.set(proxy, gate);
  proxyOfInner.set(inner, proxy);
  return proxy;
}

export function runGateOf(adapter: DockletAdapter): RunGate | undefined {
  return gateOfProxy.get(adapter);
}
