// Source: docklet-next/issues/581
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original session memory of detected Unraid hosts for M37 #581.

const detected = new Set<string>();

/**
 * `count` is the host's Unraid container count from a fresh, complete inventory, or undefined when unknown
 * (failed, partial, stale or offline). Unknown never clears a detection; only a fresh zero does.
 */
export function unraidDetected(hostId: string, count: number | undefined): boolean {
  if (count === undefined) return detected.has(hostId);
  if (count > 0) detected.add(hostId);
  else detected.delete(hostId);
  return count > 0;
}

/** Test seam: forget every detection. */
export function resetUnraidDetection() { detected.clear(); }
