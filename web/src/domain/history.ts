// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original runtime history view model for issues #335, #336 and #337.

/** Ranges of the history sections: 96 × 15 min, 84 × 2 h, 90 × 8 h. */
export type HistoryRange = "24h" | "7d" | "30d";
export const HISTORY_RANGES: readonly HistoryRange[] = ["24h", "7d", "30d"];
export const HISTORY_BUCKETS: Record<HistoryRange, { count: number; minutes: number }> = {
  "24h": { count: 96, minutes: 15 },
  "7d": { count: 84, minutes: 120 },
  "30d": { count: 90, minutes: 480 },
};

export type BucketState = "running" | "unhealthy" | "down" | "stopped" | "nodata";

export interface BucketMinutes {
  running: number;
  unhealthy: number;
  down: number;
  stopped: number;
}

export interface HistoryBucket {
  start: string;
  end: string;
  state: BucketState;
  minutes: BucketMinutes;
  /** The open bucket that is still being filled. */
  partial?: boolean;
  /** CPU as share of the host (0–100). */
  cpuAvg?: number;
  cpuMax?: number;
  memAvg?: number;
  memMax?: number;
  memLimit?: number;
  exitCode?: number;
}

export interface RuntimeHistory {
  range: HistoryRange;
  buckets: HistoryBucket[];
  /** Null when the range has no running, unhealthy or down minutes. */
  uptimePercent: number | null;
  cpuAvg?: number;
  cpuMax?: number;
  cpuNow?: number;
  memAvg?: number;
  memMax?: number;
  memLimit?: number;
  totals: { netRx: number; netTx: number; diskRead: number; diskWrite: number };
}

export interface HostCapacity {
  hostId: string;
  diskTotal: number;
  diskUsed: number;
  /** Bytes per day; absent until 24 h of data on the current disk path exist. */
  growthPerDay?: number;
  daysUntilFull?: number;
  /** Daily points of the last 30 days, oldest first. */
  series: { at: string; used: number }[];
}

export interface HistoryApi {
  /** `containerId` is the inventory id; history follows the container name. */
  getContainerHistory(containerId: string, range: HistoryRange): Promise<RuntimeHistory>;
  getStackHistory(stackId: string, range: HistoryRange): Promise<RuntimeHistory>;
  getCapacity(): Promise<HostCapacity[]>;
}

/** Colour of a bucket: the same four states as live status, plus no data. */
export const BUCKET_TONE: Record<BucketState, "ok" | "warn" | "err" | "gray" | "none"> = {
  running: "ok",
  unhealthy: "warn",
  down: "err",
  stopped: "gray",
  nodata: "none",
};
