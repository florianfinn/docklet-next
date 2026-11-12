// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original browser-wide pause clock for logical read deadlines during hub maintenance holds.

// Hub maintenance is one state per browser; a read waiting in its hold must not spend its logical deadline.
const holders = new Set<object>();
const listeners = new Set<() => void>();
let pausedBefore = 0;
let pausedSince: number | undefined;

/** Total milliseconds deadlines have been paused, including a pause still running. */
export function pausedTotal(now = Date.now()): number {
  return pausedBefore + (pausedSince === undefined ? 0 : now - pausedSince);
}

export const deadlinesPaused = () => pausedSince !== undefined;

/** Each maintenance observer reports whether it holds requests now. */
export function setDeadlinePause(owner: object, paused: boolean): void {
  const was = holders.size > 0;
  if (paused) holders.add(owner); else holders.delete(owner);
  const now = Date.now();
  if (!was && holders.size) pausedSince = now;
  if (was && !holders.size) {
    pausedBefore += now - (pausedSince ?? now);
    pausedSince = undefined;
    for (const listener of [...listeners]) listener();
  }
}

export function onDeadlinesResume(listener: () => void): () => void {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}

/**
 * A request timer that spends only active time: while deadlines are paused it parks and resumes with the remainder.
 * Returns a cancel function.
 */
export function pausableTimeout(milliseconds: number, fire: () => void): () => void {
  const started = Date.now(), pauseStart = pausedTotal(started);
  let timer: ReturnType<typeof setTimeout> | undefined;
  let offResume: (() => void) | undefined;
  let done = false;
  const check = () => {
    timer = undefined;
    if (done) return;
    if (deadlinesPaused()) { offResume = onDeadlinesResume(() => { offResume?.(); offResume = undefined; check(); }); return; }
    const now = Date.now();
    const remaining = milliseconds - (now - started - (pausedTotal(now) - pauseStart));
    if (remaining > 0) { timer = setTimeout(check, remaining); return; }
    done = true;
    fire();
  };
  timer = setTimeout(check, Math.max(0, milliseconds));
  return () => { done = true; if (timer) clearTimeout(timer); offResume?.(); };
}

/** Milliseconds of active time since `started` (paused time excluded); `pauseAtStart` is `pausedTotal(started)`. */
export function activeElapsed(started: number, pauseAtStart: number, now = Date.now()): number {
  return now - started - (pausedTotal(now) - pauseAtStart);
}

/** A clock that stands still while deadlines are paused; differences of two readings are active time. */
export const activeNow = (): number => Date.now() - pausedTotal();
