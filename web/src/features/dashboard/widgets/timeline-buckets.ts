// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original 15-minute timeline buckets per host lane.

import type { OpsEvent } from "@/domain/model";

export const TIMELINE_SPAN_MS = 24 * 3_600_000;
export const BUCKET_MS = 15 * 60_000;

/** One marker: the events of one host within one 15-minute slot, newest first. */
export type TimelineBucket = { key: string; at: string; left: number; events: OpsEvent[] };

/**
 * Groups the last 24 hours per host into at most 96 slots, counted back from `now`; future and older events are
 * left out. One pass over the events; the result is new objects and the input is not changed.
 */
export function timelineBuckets(events: readonly OpsEvent[], hostIds: readonly string[], now: number): Map<string, TimelineBucket[]> {
  const lanes = new Map<string, Map<number, OpsEvent[]>>(hostIds.map((id) => [id, new Map()]));
  for (const event of events) {
    const slots = event.hostId === undefined ? undefined : lanes.get(event.hostId);
    const age = now - Date.parse(event.at);
    if (!slots || !(age >= 0) || age >= TIMELINE_SPAN_MS) continue;
    const slot = Math.floor(age / BUCKET_MS);
    const list = slots.get(slot);
    if (list) list.push(event); else slots.set(slot, [event]);
  }
  const result = new Map<string, TimelineBucket[]>();
  for (const [hostId, slots] of lanes) {
    result.set(hostId, [...slots].sort(([a], [b]) => b - a).map(([slot, list]) => {
      const sorted = [...list].sort((a, b) => Date.parse(b.at) - Date.parse(a.at));
      // The marker sits at the slot's middle; 0 % is 24 h ago, 100 % is now.
      const left = 100 - ((slot + 0.5) * BUCKET_MS / TIMELINE_SPAN_MS) * 100;
      return { key: `${hostId}:${slot}`, at: sorted[0]!.at, left: Math.max(1, Math.min(99, left)), events: sorted };
    }));
  }
  return result;
}
