// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original owner of the Dashboard's bounded recent-events read and its stream triggers.

import { RECENT_EVENT_LIMIT } from "../adapter";
import { createSingleFlight } from "./single-flight";

/** The Dashboard reads the last 24 hours, at most 500 events, in one page. */
export const DASHBOARD_EVENT_WINDOW_MS = 24 * 60 * 60 * 1_000;
export const DASHBOARD_EVENT_LIMIT = RECENT_EVENT_LIMIT;
/** A cold read waits at most this long after the stream request for the connection's initial event frame. */
export const INITIAL_FRAME_WAIT_MS = 1_000;

/**
 * The event stream sends one frame right after its producer subscribed. A read started after that frame already
 * contains everything it signals, so it absorbs the frame; a read started earlier (before the stream, a fallback
 * after the wait, an earlier connection) is refreshed by it. Every later frame refreshes once (coalesced).
 */
export function createDashboardEvents<T>(read: (signal: AbortSignal) => Promise<T>, generation: () => number,
  { hasConsumer, changed }: { hasConsumer: () => boolean; changed: () => void }) {
  const flight = createSingleFlight(read, generation, { hasConsumer });
  let awaitingInitial = false;
  // Whether a read of this identity started before the current connection's initial frame.
  let issued = false;
  let gate: Promise<void> | undefined;
  let open: (() => void) | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  function release() {
    clearTimeout(timer); timer = undefined;
    const resolve = open; open = undefined; gate = undefined;
    resolve?.();
  }
  function refresh() { void flight.notify().catch(() => undefined); changed(); }
  return {
    async load(): Promise<T> {
      if (gate) await gate;
      issued = true;
      return flight.request();
    },
    /** A stream request started; without the events channel there is no frame to wait for. */
    streamStarted(withEvents: boolean) {
      release();
      awaitingInitial = withEvents;
      if (!withEvents) return;
      gate = new Promise<void>((resolve) => { open = resolve; });
      timer = setTimeout(release, INITIAL_FRAME_WAIT_MS);
    },
    /** An event frame arrived on the current connection. */
    frame() {
      const initial = awaitingInitial;
      awaitingInitial = false;
      release();
      if (!initial || issued) refresh();
    },
    /** Frames may have been lost (sequence gap, unreadable envelope): refresh once. */
    missed() { if (issued) refresh(); },
    identityChanged() {
      release();
      awaitingInitial = false;
      issued = false;
      flight.identityChanged();
    },
  };
}
