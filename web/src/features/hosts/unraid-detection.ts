// Source: docklet-next/issues/581
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original sign-in-scoped memory of detected Unraid hosts for M37 #581.

/** Detections of the current sign-in only: a new identity version starts empty, so no user sees another user's detection. */
let owner: number | undefined;
const detected = new Set<string>();

/**
 * `count` is the host's Unraid container count from a fresh, complete inventory, or undefined when unknown
 * (failed, partial, stale or offline). Unknown never clears a detection; only a fresh zero or the host's removal does.
 */
export function unraidDetected(identity: number, hostId: string, count: number | undefined): boolean {
  if (owner !== identity) { owner = identity; detected.clear(); }
  if (count === undefined) return detected.has(hostId);
  if (count > 0) detected.add(hostId);
  else detected.delete(hostId);
  return count > 0;
}

/** A confirmed removal ends the host's lifetime; a later host with the same ID starts undetected. */
export function forgetUnraidDetection(hostId: string) { detected.delete(hostId); }

/** Test seam: forget every detection. */
export function resetUnraidDetection() { owner = undefined; detected.clear(); }
