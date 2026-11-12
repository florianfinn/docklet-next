// Source: docklet-next/issues/400
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original time-placed capacity line segments for the Dashboard storage forecast (#400).

import { HISTORY_BUCKETS, type HostCapacity } from "@/domain/history";

/** One 30-day bucket (8 hours). */
const STEP_MS = HISTORY_BUCKETS["30d"].minutes * 60_000;

/** Line segments of the 30-day usage, placed by bucket time; a missing bucket between two points breaks the line. */
export function capacitySegments(trend: Pick<HostCapacity, "series" | "diskTotal" | "domain">): Array<Array<[number, number]>> {
  const { series, diskTotal: total } = trend;
  if (!series.length || total <= 0) return [];
  const start = trend.domain ? Date.parse(trend.domain.start) : Date.parse(series[0]!.at) - STEP_MS;
  const end = trend.domain ? Date.parse(trend.domain.end) : Date.parse(series.at(-1)!.at);
  const span = Math.max(1, end - start);
  const segments: Array<Array<[number, number]>> = [];
  let previous: number | undefined;
  for (const point of series) {
    const at = Date.parse(point.at);
    if (previous === undefined || at - previous > STEP_MS * 1.5) segments.push([]);
    segments.at(-1)!.push([((at - start) / span) * 100, (1 - Math.min(1, point.used / total)) * 24]);
    previous = at;
  }
  return segments;
}
