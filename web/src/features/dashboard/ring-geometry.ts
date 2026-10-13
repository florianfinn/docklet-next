// Pure geometry of the dashboard fleet ring.

export interface RingSegment<T> {
  item: T;
  /** Degrees clockwise from 12 o'clock. */
  start: number;
  end: number;
  index: number;
}

export interface RingGroup<G, T> {
  group: G;
  start: number;
  end: number;
  segments: RingSegment<T>[];
}

/** Gap between host groups and between neighbouring segments, in degrees. */
const GROUP_GAP = 8;
const SEGMENT_GAP = 0.9;

export function layoutRing<G, T>(groups: { group: G; items: T[] }[]): RingGroup<G, T>[] {
  const filled = groups.filter((g) => g.items.length);
  const total = filled.reduce((n, g) => n + g.items.length, 0);
  const gap = filled.length > 1 ? GROUP_GAP : 0;
  const per = total ? (360 - gap * filled.length) / total : 0;
  const out: RingGroup<G, T>[] = [];
  let cursor = gap / 2;
  let index = 0;
  for (const g of filled) {
    const start = cursor;
    const segments: RingSegment<T>[] = [];
    for (const item of g.items) {
      const s = cursor + SEGMENT_GAP;
      segments.push({ item, start: s, end: Math.max(s + 0.5, cursor + per - SEGMENT_GAP), index: index++ });
      cursor += per;
    }
    out.push({ group: g.group, start, end: cursor, segments });
    cursor += gap;
  }
  return out;
}

/**
 * Radii and strokes for a ring of `size` px, proportional to the 220 px
 * reference (segments 30 px in from the edge, 14 px stroke, host arc outside).
 */
export function ringGeometry(size: number) {
  const k = size / 220;
  return {
    segR: size / 2 - 30 * k,
    segW: Math.max(8, 14 * k),
    hostR: size / 2 - 14 * k,
    hostW: Math.max(2, 3 * k),
    /** Radial distance a hovered segment moves outward. */
    fan: Math.max(4, 7 * k),
    /** Extra reach of a segment's hit area beyond its drawn band. */
    hitPad: Math.max(3, 3 * k),
    valueSize: Math.max(16, 30 * k),
    captionSize: Math.max(10, 11 * Math.min(1, k * 1.1)),
  };
}

/** Outward translation along the segment's middle angle (12 o'clock = 0°, clockwise). */
export function fanOffset(deg: number, distance: number): { x: number; y: number } {
  const rad = (deg * Math.PI) / 180;
  return { x: Number((Math.sin(rad) * distance).toFixed(2)), y: Number((-Math.cos(rad) * distance).toFixed(2)) };
}

/**
 * Fan offset of a segment. Wider than 180° the far side would move inward
 * toward the centre (a single-stack host ring would slide sideways), so such
 * segments stay in place.
 */
export function segmentFan(start: number, end: number, distance: number): { x: number; y: number } {
  return end - start > 180 ? { x: 0, y: 0 } : fanOffset((start + end) / 2, distance);
}

export function polar(cx: number, cy: number, r: number, deg: number) {
  const rad = ((deg - 90) * Math.PI) / 180;
  return { x: cx + r * Math.cos(rad), y: cy + r * Math.sin(rad) };
}

export function arcPath(cx: number, cy: number, r: number, start: number, end: number): string {
  const a = polar(cx, cy, r, start);
  const b = polar(cx, cy, r, end);
  const large = end - start > 180 ? 1 : 0;
  return `M${a.x.toFixed(2)} ${a.y.toFixed(2)} A${r} ${r} 0 ${large} 1 ${b.x.toFixed(2)} ${b.y.toFixed(2)}`;
}

/**
 * Glow filter region in user space: the whole ring plus room for the blur.
 * Bounding-box units would follow each arc's fill box, which excludes the
 * stroke and is only a few pixels tall near 6 and 12 o'clock.
 */
export function glowRegion(size: number, blur: number) {
  const margin = Math.ceil(blur * 3);
  return { x: -margin, y: -margin, width: size + 2 * margin, height: size + 2 * margin };
}

/** Axis-aligned box of a stroked arc, sampled along the arc (stroke-linecap butt). */
export function strokedArcBox(cx: number, cy: number, r: number, start: number, end: number, strokeWidth: number) {
  const xs: number[] = [];
  const ys: number[] = [];
  const steps = Math.max(8, Math.ceil(end - start));
  for (let i = 0; i <= steps; i++) {
    const deg = start + ((end - start) * i) / steps;
    for (const rr of [r - strokeWidth / 2, r + strokeWidth / 2]) {
      const p = polar(cx, cy, rr, deg);
      xs.push(p.x);
      ys.push(p.y);
    }
  }
  return { x: Math.min(...xs), y: Math.min(...ys), width: Math.max(...xs) - Math.min(...xs), height: Math.max(...ys) - Math.min(...ys) };
}

/**
 * Closed annular sector between radii r0 < r1. A span of 360° or more is a
 * full annulus of two circles; draw it with fill-rule evenodd.
 */
export function sectorPath(cx: number, cy: number, r0: number, r1: number, start: number, end: number): string {
  const f = (n: number) => n.toFixed(2);
  if (end - start >= 360 - 1e-6) {
    const circle = (r: number) => `M${f(cx)} ${f(cy - r)} A${r} ${r} 0 1 1 ${f(cx)} ${f(cy + r)} A${r} ${r} 0 1 1 ${f(cx)} ${f(cy - r)} Z`;
    return `${circle(r1)} ${circle(r0)}`;
  }
  const large = end - start > 180 ? 1 : 0;
  const a = polar(cx, cy, r1, start);
  const b = polar(cx, cy, r1, end);
  const d = polar(cx, cy, r0, end);
  const e = polar(cx, cy, r0, start);
  return `M${f(a.x)} ${f(a.y)} A${r1} ${r1} 0 ${large} 1 ${f(b.x)} ${f(b.y)} L${f(d.x)} ${f(d.y)} A${r0} ${r0} 0 ${large} 0 ${f(e.x)} ${f(e.y)} Z`;
}

export interface HitCell {
  start: number;
  end: number;
  index: number;
}

/**
 * Angular hit cells of ring segments in ring order. Neighbours of one host
 * group split their small gap in the middle, so every point belongs to the
 * nearest segment. At a gap between host groups each side reaches out by at
 * most `padDeg` and never past the gap's middle, so the gap itself stays empty.
 * A fanned segment (see segmentFan) only moves outward and toward its own
 * middle angle, so it stays inside its cell.
 */
export function hitCells(segments: readonly { start: number; end: number; index: number }[], padDeg: number): HitCell[] {
  const n = segments.length;
  const reach = (gap: number) => (gap <= 2 * SEGMENT_GAP + 1e-6 ? gap / 2 : Math.min(padDeg, gap / 2));
  return segments.map((seg, i) => {
    const next = segments[(i + 1) % n]!;
    const prev = segments[(i - 1 + n) % n]!;
    const after = i === n - 1 ? next.start + 360 - seg.end : next.start - seg.end;
    const before = i === 0 ? seg.start + 360 - prev.end : seg.start - prev.end;
    return { start: seg.start - reach(before), end: seg.end + reach(after), index: seg.index };
  });
}

/** Radial band of the hit areas: the drawn band, its fanned position and the padding on both sides. */
export function hitBand(g: ReturnType<typeof ringGeometry>, fan: boolean) {
  return { r0: g.segR - g.segW / 2 - g.hitPad, r1: g.segR + g.segW / 2 + (fan ? g.fan : 0) + g.hitPad };
}

/** Index of the hit cell under a point, or undefined in the centre, outside or in a gap between host groups. */
export function cellAt(cells: readonly HitCell[], band: { r0: number; r1: number }, cx: number, cy: number, x: number, y: number): number | undefined {
  const r = Math.hypot(x - cx, y - cy);
  if (r < band.r0 || r > band.r1) return undefined;
  const deg = ((Math.atan2(x - cx, cy - y) * 180) / Math.PI + 360) % 360;
  return cells.find((c) => [deg, deg + 360, deg - 360].some((d) => d >= c.start && d <= c.end))?.index;
}
