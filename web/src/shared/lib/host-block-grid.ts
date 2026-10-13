import type { CSSProperties } from "react";

/**
 * Density limit for host blocks: tracks of at least (row - gaps) / max, so no
 * further column fits, and a grid no wider than `count` blocks of `maxWidth`,
 * so one or two hosts keep a bounded width instead of filling the row. The
 * track maximum stays 1fr: auto-fit counts columns by a definite maximum, which
 * would drop columns on mid-size screens. gap-4 is 1rem.
 */
export function cappedGrid(max: number, count: number, maxWidth?: string): CSSProperties {
  const n = Math.max(1, Math.min(max, count));
  return {
    gridTemplateColumns: `repeat(auto-fit, minmax(min(100%, max(var(--hb-min), calc((100% - ${max - 1}rem) / ${max}))), 1fr))`,
    maxWidth: maxWidth ? `calc(${n} * ${maxWidth} + ${n - 1}rem)` : undefined,
  };
}
