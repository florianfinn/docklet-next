// Source: docklet-next/issues/509
// Revision: fb1306767563a1e0d7bd028aaef362cca38166b6
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original display smoothing for host card history.

/** Weight of the newest sample: a single spike shows at 40 %, a sustained step reaches about 90 % within five samples. */
export const HISTORY_SMOOTHING = 0.4;

/** Exponentially smoothed copy of a metric history for display; the samples themselves stay unchanged. */
export function smoothHistory(values: readonly number[], alpha = HISTORY_SMOOTHING): number[] {
  const result: number[] = [];
  for (const value of values) {
    const previous = result.at(-1);
    result.push(previous === undefined ? value : previous + alpha * (value - previous));
  }
  return result;
}
