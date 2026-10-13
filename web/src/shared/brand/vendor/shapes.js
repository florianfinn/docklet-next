// Shape helpers and easing curves shared by the candidate definitions.
// Parts are plain data: { part, tag, paint, group?, draw?, attrs }.

import { num } from './glyphs.js';

export const OUT = 'cubic-bezier(0.16, 1, 0.3, 1)';
export const IN_OUT = 'cubic-bezier(0.65, 0, 0.35, 1)';
export const FALL = 'cubic-bezier(0.55, 0, 0.9, 0.45)';

export const rect = (part, x, y, width, height, rx, group) => ({ part, tag: 'rect', paint: 'fill', group, attrs: { x, y, width, height, rx } });
export const path = (part, d, group) => ({ part, tag: 'path', paint: 'fill', group, attrs: { d } });
export const circle = (part, cx, cy, r, group) => ({ part, tag: 'circle', paint: 'fill', group, attrs: { cx, cy, r } });

/** Stroked part; `round` adds round caps and joins. */
export const band = (part, d, width, { round = false, group } = {}) => ({
  part,
  tag: 'path',
  paint: 'stroke',
  draw: true,
  group,
  attrs: { d, 'stroke-width': width, ...(round ? { 'stroke-linecap': 'round', 'stroke-linejoin': 'round' } : {}) },
});

/** Rounded rectangle as a clockwise path that starts after the top-left corner. */
export function rectPath(x, y, w, h, r = 0) {
  const q = Math.min(r, w / 2, h / 2);
  if (!q) return `M${num(x)} ${num(y)} H${num(x + w)} V${num(y + h)} H${num(x)} Z`;
  const arc = (ex, ey) => `A${num(q)} ${num(q)} 0 0 1 ${num(ex)} ${num(ey)}`;
  return [
    `M${num(x + q)} ${num(y)} H${num(x + w - q)}`,
    arc(x + w, y + q),
    `V${num(y + h - q)}`,
    arc(x + w - q, y + h),
    `H${num(x + q)}`,
    arc(x, y + h - q),
    `V${num(y + q)}`,
    arc(x + q, y),
    'Z',
  ].join(' ');
}

/** Circle as a clockwise path starting at its leftmost point. */
export function circlePath(cx, cy, r) {
  const a = `A${num(r)} ${num(r)} 0 1 1`;
  return `M${num(cx - r)} ${num(cy)} ${a} ${num(cx + r)} ${num(cy)} ${a} ${num(cx - r)} ${num(cy)} Z`;
}

/** Square of side `s` turned by 45°, centred on (cx, cy), with corner radius `r`. */
export function diamondPath(cx, cy, s, r) {
  const h = s / Math.SQRT2;
  const k = r / Math.SQRT2;
  const corners = [
    [cx, cy - h],
    [cx + h, cy],
    [cx, cy + h],
    [cx - h, cy],
  ];
  // Unit edge directions, clockwise: top→right, right→bottom, bottom→left, left→top.
  const dirs = [
    [1, 1],
    [-1, 1],
    [-1, -1],
    [1, -1],
  ];
  const out = [];
  corners.forEach(([x, y], i) => {
    const before = dirs[(i + 3) % 4];
    const after = dirs[i];
    const a = [x - before[0] * k, y - before[1] * k];
    const b = [x + after[0] * k, y + after[1] * k];
    out.push(`${i ? 'L' : 'M'}${num(a[0])} ${num(a[1])}`, `A${num(r)} ${num(r)} 0 0 1 ${num(b[0])} ${num(b[1])}`);
  });
  return `${out.join(' ')} Z`;
}

/** Outline of any part as path data (used by the blueprint construction layer). */
export function outlinePath(part) {
  const a = part.attrs;
  if (part.tag === 'path') return a.d;
  if (part.tag === 'circle') return circlePath(a.cx, a.cy, a.r);
  return rectPath(a.x, a.y, a.width, a.height, a.rx ?? 0);
}

export const slideIn = (dx, overshoot) => [
  { offset: 0, opacity: 0, transform: `translateX(${dx}px)`, easing: OUT },
  { offset: 0.3, opacity: 1 },
  { offset: 0.72, transform: `translateX(${overshoot}px)`, easing: IN_OUT },
  { offset: 1, opacity: 1, transform: 'none' },
];

export const dropIn = (dy) => [
  { offset: 0, opacity: 0, transform: `translateY(${dy}px)`, easing: FALL },
  { offset: 0.2, opacity: 1 },
  { offset: 0.6, transform: 'translateY(0px) scaleY(0.86)', easing: OUT },
  { offset: 0.82, transform: 'translateY(-0.8px) scaleY(1.02)', easing: IN_OUT },
  { offset: 1, opacity: 1, transform: 'none' },
];

export const popIn = (from = 0) => [
  { offset: 0, opacity: 0, transform: `scale(${from})`, easing: OUT },
  { offset: 0.35, opacity: 1 },
  { offset: 0.7, transform: 'scale(1.06)', easing: IN_OUT },
  { offset: 1, opacity: 1, transform: 'none' },
];

/** Short dip used when a module lands on its base. */
export const settle = (amount = 0.97) => [
  { offset: 0, transform: 'none', easing: OUT },
  { offset: 0.35, transform: `scaleY(${amount})`, easing: IN_OUT },
  { offset: 1, transform: 'none' },
];
