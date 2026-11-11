// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original deterministic demo history for issues #335, #336 and #337.

import { HISTORY_BUCKETS, type BucketState, type HistoryApi, type HistoryBucket, type HistoryRange, type HostCapacity, type RuntimeHistory } from "@/domain/history";

/** Small deterministic generator so demo bars stay stable between renders. */
function seeded(text: string) {
  let h = 2166136261;
  for (const ch of text) h = Math.imul(h ^ ch.charCodeAt(0), 16777619);
  return () => {
    h = Math.imul(h ^ (h >>> 15), 2246822507) ^ Math.imul(h ^ (h >>> 13), 3266489909);
    return ((h >>> 0) % 10000) / 10000;
  };
}

export function demoHistory(key: string, range: HistoryRange, now = Date.now()): RuntimeHistory {
  const { count, minutes } = HISTORY_BUCKETS[range];
  const rnd = seeded(`${key}:${range}`);
  const ms = minutes * 60_000;
  const end = Math.floor(now / ms) * ms + ms;
  const buckets: HistoryBucket[] = Array.from({ length: count }, (_, i) => {
    const start = end - (count - i) * ms;
    const r = rnd();
    const state: BucketState = i < count * 0.08 ? "nodata" : r < 0.015 ? "down" : r < 0.05 ? "unhealthy" : r < 0.08 ? "stopped" : "running";
    const lost = state === "down" || state === "unhealthy" ? Math.max(1, Math.round(minutes * rnd() * 0.4)) : 0;
    const bucketMinutes = { running: state === "nodata" || state === "stopped" ? 0 : minutes - lost, unhealthy: state === "unhealthy" ? lost : 0, down: state === "down" ? lost : 0, stopped: state === "stopped" ? minutes : 0 };
    const cpuAvg = state === "running" || state === "unhealthy" ? 2 + rnd() * 18 : undefined;
    return {
      start: new Date(start).toISOString(),
      end: new Date(start + ms).toISOString(),
      state,
      minutes: bucketMinutes,
      ...(i === count - 1 ? { partial: true } : {}),
      ...(cpuAvg === undefined ? {} : { cpuAvg, cpuMax: cpuAvg * (1.3 + rnd()), memAvg: (300 + rnd() * 200) * 2 ** 20, memMax: 520 * 2 ** 20, memLimit: 1024 * 2 ** 20 }),
      ...(state === "down" ? { exitCode: 1 } : {}),
    };
  });
  const counted = buckets.reduce((n, b) => ({ running: n.running + b.minutes.running, unhealthy: n.unhealthy + b.minutes.unhealthy, down: n.down + b.minutes.down }), { running: 0, unhealthy: 0, down: 0 });
  const denominator = counted.running + counted.unhealthy + counted.down;
  const withCpu = buckets.filter((b) => b.cpuAvg !== undefined);
  return {
    range,
    buckets,
    uptimePercent: denominator ? ((counted.running + counted.unhealthy / 2) / denominator) * 100 : null,
    cpuAvg: withCpu.reduce((n, b) => n + b.cpuAvg!, 0) / Math.max(1, withCpu.length),
    cpuMax: Math.max(0, ...withCpu.map((b) => b.cpuMax!)),
    cpuNow: withCpu.at(-1)?.cpuAvg,
    memAvg: withCpu.reduce((n, b) => n + b.memAvg!, 0) / Math.max(1, withCpu.length),
    memMax: Math.max(0, ...withCpu.map((b) => b.memMax!)),
    memLimit: 1024 * 2 ** 20,
    totals: { netRx: 280e6, netTx: 60e6, diskRead: 1.1e9, diskWrite: 510e6 },
  };
}

export function demoCapacity(hostIds: readonly string[], now = Date.now()): HostCapacity[] {
  const stepMs = HISTORY_BUCKETS["30d"].minutes * 60_000;
  const buckets = HISTORY_BUCKETS["30d"].count;
  const first = Math.floor(now / stepMs) * stepMs - (buckets - 1) * stepMs;
  return hostIds.map((hostId) => {
    const rnd = seeded(`capacity:${hostId}`);
    const diskTotal = (500 + Math.round(rnd() * 3) * 500) * 1e9;
    const growthPerDay = (0.2 + rnd() * 3) * 1e9;
    const diskUsed = diskTotal * (0.3 + rnd() * 0.5);
    return {
      hostId,
      diskTotal,
      diskUsed,
      growthPerDay,
      daysUntilFull: (diskTotal - diskUsed) / growthPerDay,
      // The hub's 30-day shape: 90 buckets of 8 hours and their domain.
      series: Array.from({ length: buckets }, (_, i) => ({ at: new Date(first + i * stepMs).toISOString(), used: Math.max(0, diskUsed - ((buckets - 1 - i) * growthPerDay) / 3) })),
      domain: { start: new Date(first).toISOString(), end: new Date(first + buckets * stepMs).toISOString() },
    };
  });
}

export function createDemoHistory(hostIds: () => readonly string[]): HistoryApi {
  return {
    getContainerHistory: async ({ hostId, name }, range) => demoHistory(`c:${hostId}:${name}`, range),
    getStackHistory: async (stackId, range) => demoHistory(`s:${stackId}`, range),
    getCapacity: async () => demoCapacity(hostIds()),
  };
}
