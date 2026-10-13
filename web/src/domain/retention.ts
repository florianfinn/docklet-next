// How many backup archives a target keeps. Two independent limits that
// combine: an older archive is removed as soon as keeping it would exceed
// either the archive count or the total size. Both off means unlimited.

export interface Retention {
  /** Keep at most this many archives; null = no count limit. */
  maxCount: number | null;
  /** Keep at most this total size in GB; null = no size limit. */
  maxSizeGb: number | null;
}

export const RETENTION_LIMITS = {
  count: { min: 1, max: 999, slider: 60, fallback: 7 },
  size: { min: 1, max: 100_000, slider: 500, fallback: 50 },
} as const;

export const DEFAULT_RETENTION: Retention = { maxCount: 7, maxSizeGb: null };

export type RetentionError = "count" | "size";

const GB = 1024 ** 3;

/**
 * Accepts the current shape and the former plain archive count, so stored
 * plans and settings keep their meaning; anything else falls back.
 */
export function normalizeRetention(value: unknown, fallback: Retention = DEFAULT_RETENTION): Retention {
  if (typeof value === "number" && Number.isFinite(value)) return { maxCount: Math.round(value), maxSizeGb: null };
  if (!value || typeof value !== "object") return fallback;
  const v = value as Partial<Record<keyof Retention, unknown>>;
  const num = (x: unknown) => (x === null ? null : typeof x === "number" && Number.isFinite(x) ? x : undefined);
  const maxCount = num(v.maxCount);
  const maxSizeGb = num(v.maxSizeGb);
  return { maxCount: maxCount === undefined ? fallback.maxCount : maxCount, maxSizeGb: maxSizeGb === undefined ? fallback.maxSizeGb : maxSizeGb };
}

export function validateRetention(r: Retention): RetentionError | null {
  const ok = (v: number, lim: { min: number; max: number }) => Number.isInteger(v) && v >= lim.min && v <= lim.max;
  if (r.maxCount !== null && !ok(r.maxCount, RETENTION_LIMITS.count)) return "count";
  if (r.maxSizeGb !== null && !ok(r.maxSizeGb, RETENTION_LIMITS.size)) return "size";
  return null;
}

/**
 * Splits archives (newest first) into kept and removed ones. Kept archives
 * are always the newest ones without gaps: once a limit is reached, every
 * older archive goes. The newest archive always stays, so a small size limit
 * never deletes the run that just finished; unknown sizes count as 0 bytes.
 */
export function applyRetention<T extends { sizeBytes?: number }>(newestFirst: readonly T[], r: Retention): { keep: T[]; drop: T[] } {
  const keep: T[] = [];
  const drop: T[] = [];
  let bytes = 0;
  for (const run of newestFirst) {
    const next = bytes + (run.sizeBytes ?? 0);
    const overCount = r.maxCount !== null && keep.length >= r.maxCount;
    const overSize = r.maxSizeGb !== null && next > r.maxSizeGb * GB;
    if (drop.length || (keep.length > 0 && (overCount || overSize))) drop.push(run);
    else {
      keep.push(run);
      bytes = next;
    }
  }
  return { keep, drop };
}
