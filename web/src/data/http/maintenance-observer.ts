// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original browser-wide hub maintenance observer and bounded read hold.

import { setDeadlinePause } from "./deadline-pause";

export type MaintenanceState = { active: boolean; operation?: "backup" | "restore"; overdue: boolean };
export type HoldOutcome = "released" | "overflow";

/** At most this many requests wait; later ones settle at once with the maintenance answer. */
export const MAINTENANCE_WAITERS = 64;
/** A hold never outlasts this; afterwards requests proceed and surface their own errors. */
export const MAINTENANCE_MAX_HOLD_MS = 120_000;
const ACTIVE_POLL_MS = 1_000;
const BACKOFF_MS = [1_000, 2_000, 4_000, 5_000];

const cancelled = () => { const controller = new AbortController(); controller.abort(); return controller.signal.reason as unknown; };
type Waiter = { resolve: (outcome: HoldOutcome) => void; reject: (reason: unknown) => void; signal?: AbortSignal; abort: () => void };

/** The status endpoint is read on the raw fetcher: no CSRF bootstrap, no hold, no host admission. */
export function createMaintenanceObserver(statusUrl: string, fetcher: typeof fetch) {
  let state: MaintenanceState = { active: false, overdue: false };
  let holdTimer: ReturnType<typeof setTimeout> | undefined;
  let pollTimer: ReturnType<typeof setTimeout> | undefined;
  let polling = false;
  let failures = 0;
  let epoch = 0;
  const waiters = new Set<Waiter>();
  const listeners = new Set<(next: MaintenanceState) => void>();

  const owner = {};
  function publish(next: MaintenanceState) {
    if (next.active === state.active && next.operation === state.operation && next.overdue === state.overdue) return;
    state = next;
    setDeadlinePause(owner, state.active && !state.overdue);
    listeners.forEach((listener) => listener(state));
  }
  function release() {
    for (const waiter of waiters) { waiter.signal?.removeEventListener("abort", waiter.abort); waiter.resolve("released"); }
    waiters.clear();
  }
  function end() {
    if (holdTimer) clearTimeout(holdTimer);
    holdTimer = undefined;
    release();
    publish({ active: false, overdue: false });
  }
  function begin(operation?: "backup" | "restore") {
    if (!state.active) {
      holdTimer = setTimeout(() => { holdTimer = undefined; release(); publish({ ...state, overdue: true }); }, MAINTENANCE_MAX_HOLD_MS);
    }
    publish({ active: true, operation: operation ?? state.operation, overdue: state.overdue });
  }
  function schedule(delay: number) {
    if (pollTimer) clearTimeout(pollTimer);
    pollTimer = setTimeout(() => { pollTimer = undefined; void poll(); }, delay);
  }
  async function poll() {
    if (polling) return;
    polling = true;
    const version = epoch;
    let next: "active" | "inactive" | "unknown" = "unknown";
    let operation: "backup" | "restore" | undefined;
    try {
      const response = await fetcher(statusUrl, { credentials: "same-origin", headers: { Accept: "application/json" }, cache: "no-store" });
      // A revoked or refused session ends the status read; held reads then meet their own answer.
      if (response.status === 401 || response.status === 403) { void response.body?.cancel().catch(() => undefined); next = "inactive"; }
      else if (response.ok) {
        const body = await response.json() as Record<string, unknown>;
        const value = body.data && typeof body.data === "object" ? body.data as Record<string, unknown> : body;
        if (typeof value.active === "boolean") {
          next = value.active ? "active" : "inactive";
          operation = value.operation === "backup" ? "backup" : value.operation === "restore" ? "restore" : undefined;
        }
      } else void response.body?.cancel().catch(() => undefined);
    } catch { next = "unknown"; }
    polling = false;
    if (version !== epoch) return;
    if (next === "inactive") { failures = 0; end(); return; }
    if (next === "active") { failures = 0; begin(operation); schedule(ACTIVE_POLL_MS); return; }
    schedule(BACKOFF_MS[Math.min(failures++, BACKOFF_MS.length - 1)]!);
  }

  return {
    state: () => state,
    subscribe(listener: (next: MaintenanceState) => void) { listeners.add(listener); return () => { listeners.delete(listener); }; },
    /** A held-class request met the maintenance answer: hold from now and confirm through the status endpoint. */
    signal() {
      begin();
      if (!pollTimer && !polling) void poll();
    },
    /** A maintenance status read elsewhere (the hub backup page) is the same observation. */
    observe(status: { active?: unknown; operation?: unknown }) {
      if (status.active === true) {
        begin(status.operation === "backup" ? "backup" : status.operation === "restore" ? "restore" : undefined);
        if (!pollTimer && !polling) schedule(ACTIVE_POLL_MS);
      } else if (status.active === false) end();
    },
    /** Whether a held-class request must wait now; an overdue hold lets requests through. */
    holding: () => state.active && !state.overdue,
    /** Wait until maintenance ends or the hold expires; the request's own abort rejects only this waiter. */
    wait(signal?: AbortSignal): Promise<HoldOutcome> {
      if (!state.active || state.overdue) return Promise.resolve("released");
      if (waiters.size >= MAINTENANCE_WAITERS) return Promise.resolve("overflow");
      signal?.throwIfAborted();
      return new Promise<HoldOutcome>((resolve, reject) => {
        const waiter: Waiter = { resolve, reject, signal, abort: () => { waiters.delete(waiter); reject(signal?.reason); } };
        waiters.add(waiter);
        signal?.addEventListener("abort", waiter.abort, { once: true });
      });
    },
    waiting: () => waiters.size,
    /** Identity reset: forget the observed state and reject every waiter; none of them is dispatched afterwards. */
    reset() {
      ++epoch;
      if (pollTimer) clearTimeout(pollTimer);
      pollTimer = undefined;
      polling = false;
      failures = 0;
      for (const waiter of waiters) { waiter.signal?.removeEventListener("abort", waiter.abort); waiter.reject(cancelled()); }
      waiters.clear();
      end();
    },
  };
}

export type MaintenanceObserver = ReturnType<typeof createMaintenanceObserver>;
