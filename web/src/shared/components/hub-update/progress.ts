// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original update-view progress curve (fixed estimate, capped before the result).


export const UPDATE_ESTIMATE = 60_000;
export const UPDATE_CLOSE_AFTER = 20 * 60_000;
export function updateProgress(elapsed: number) {
  const t = Math.max(0, elapsed);
  return Math.min(99, t <= UPDATE_ESTIMATE ? 90 * t / UPDATE_ESTIMATE : 90 + 9 * (1 - Math.exp(-(t - UPDATE_ESTIMATE) / UPDATE_ESTIMATE)));
}
