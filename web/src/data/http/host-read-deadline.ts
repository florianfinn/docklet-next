// Source: docklet-next/issues/271
// Revision: 4e71e45a2297a4f2719188cdf15c3e4d586b5357
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original logical host read budgets covering queuing, bodies and decoration. M23 #391: hub maintenance holds pause the budgets.

import { CancelledError } from "@tanstack/react-query";
import { AdapterOutcomeError } from "../capability";
import { deadlinesPaused, onDeadlinesResume, pausedTotal } from "./deadline-pause";

/** Settled bodies are never aborted; cancellation rejects even an uncooperative transport. */
export function hostReadDeadline(milliseconds: number, code: string) {
  return async <T>(read: (signal: AbortSignal) => Promise<T>, parent?: AbortSignal): Promise<T> => {
    const controller = new AbortController();
    let reject!: (error: unknown) => void;
    const stopped = new Promise<never>((_, fail) => { reject = fail; });
    const stop = (error: unknown) => { controller.abort(error); reject(error); };
    const abort = () => stop(parent?.reason instanceof AdapterOutcomeError ? parent.reason : new CancelledError());
    // Time spent in a hub maintenance hold does not count against the budget.
    const pauseStart = pausedTotal();
    let granted = 0;
    let offResume: (() => void) | undefined;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const expire = () => {
      timer = undefined;
      if (deadlinesPaused()) { offResume = onDeadlinesResume(() => { offResume?.(); offResume = undefined; expire(); }); return; }
      const extra = pausedTotal() - pauseStart - granted;
      if (extra > 0) { granted += extra; timer = setTimeout(expire, extra); return; }
      stop(new AdapterOutcomeError({ kind: "error", code }));
    };
    if (milliseconds > 0) timer = setTimeout(expire, milliseconds);
    parent?.addEventListener("abort", abort, { once: true });
    if (parent?.aborted) abort();
    else if (milliseconds <= 0) stop(new AdapterOutcomeError({ kind: "error", code }));
    try {
      return await Promise.race([controller.signal.aborted ? stopped : read(controller.signal), stopped]);
    } catch (error) { controller.abort(error); throw error; }
    finally { if (timer) clearTimeout(timer); offResume?.(); parent?.removeEventListener("abort", abort); }
  };
}

/** Capture before prerequisites or browser queuing; every host shares the query's start time. */
export function inventoryReadBudget(parent?: AbortSignal, code = "http.error") {
  const started = Date.now(), paused = pausedTotal(started);
  return <T>(read: (signal: AbortSignal) => Promise<T>) => {
    const now = Date.now();
    return hostReadDeadline(30_000 - (now - started - (pausedTotal(now) - paused)), code)(read, parent);
  };
}
