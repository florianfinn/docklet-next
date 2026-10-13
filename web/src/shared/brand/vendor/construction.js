// Blueprint construction layer: guide lines, thin outlines, clipped inking
// strokes and a thin letter skeleton drawn on top of the logo. Every element
// rests at opacity 0, so the settled logo is exactly the static SVG and
// reduced motion shows no construction at all. logo-motion.js animates it.
//
// Line widths are set for a logo CONSTRUCTION_REF px high and scale with
// --dln-k (playEntrance sets it from the rendered height).

import { num } from './glyphs.js';
import { letteringFor, runMetrics, runStrokeAttrs, runStyle, runTransform } from './lettering.js';
import { circlePath, outlinePath } from './shapes.js';

export const CONSTRUCTION_REF = 64;
const LINE_PX = { guide: 0.75, contour: 1.15, skeleton: 0.9 };
const CONTOUR_MAX = { regular: 0.9, small: 0.45 };

/** Start and end widths of the inking stroke, in mark units (inside half is visible). */
export const TRACE_WIDTH = { regular: { thin: 0.5, thick: 4 }, small: { thin: 0.3, thick: 2 } };

/** Guides of a mark; the 16 px master falls back to its ink frame. */
export function markGuides(candidate, optical) {
  if (optical === 'regular' && candidate.mark.guides) return candidate.mark.guides;
  const [x0, y0, x1, y1] = candidate.mark.ink[optical];
  return [{ h: y0 }, { h: y1 }, { v: x0 }, { v: x1 }];
}

/** Number of guide lines a variant draws (mark guides first, then text lines). */
export function guideCount(candidate, variant, optical) {
  const mark = variant === 'wordmark' ? 0 : markGuides(candidate, optical).length;
  if (variant === 'mark') return mark;
  const wm = letteringFor(candidate, variant);
  return mark + runMetrics(runStyle(wm, 'main').set).length + runMetrics(runStyle(wm, 'next').set).length;
}

/** Clips a segment to a box (Liang–Barsky); a segment outside collapses to a point. */
function clipSegment([x1, y1, x2, y2], [bx0, by0, bx1, by1]) {
  const dx = x2 - x1;
  const dy = y2 - y1;
  let t0 = 0;
  let t1 = 1;
  for (const [p, q] of [
    [-dx, x1 - bx0],
    [dx, bx1 - x1],
    [-dy, y1 - by0],
    [dy, by1 - y1],
  ]) {
    if (p === 0) {
      if (q < 0) return [x1, y1, x1, y1];
      continue;
    }
    const t = q / p;
    if (p < 0) t0 = Math.max(t0, t);
    else t1 = Math.min(t1, t);
  }
  if (t0 > t1) return [x1, y1, x1, y1];
  return [x1 + t0 * dx, y1 + t0 * dy, x1 + t1 * dx, y1 + t1 * dy];
}

const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));

function guidePath(guide, bounds) {
  const [x0, y0, x1, y1] = bounds;
  if (guide.h != null) return `M${num(x0)} ${num(clamp(guide.h, y0, y1))} H${num(x1)}`;
  if (guide.v != null) return `M${num(clamp(guide.v, x0, x1))} ${num(y0)} V${num(y1)}`;
  if (guide.circle) return circlePath(...guide.circle);
  const [a, b, c, d] = clipSegment(guide.line, bounds);
  return `M${num(a)} ${num(b)} L${num(c)} ${num(d)}`;
}

const width = (w) => `style="stroke-width:calc(var(--dln-k, 1) * ${num(w)}px)"`;
const hidden = 'pathLength="1" opacity="0"';

/**
 * Markup of the construction layer.
 *
 * - mark: { candidate, optical, transform, scale, bounds } in mark units, or null
 * - text: { wm, runs, lean, dx, dy, bounds } in root units, or null
 * - unitsPerPx: viewBox units per CSS px at CONSTRUCTION_REF
 */
export function constructionMarkup({ mark, text, unitsPerPx, idPrefix }) {
  const groups = [];
  const defs = [];
  let index = 0;

  if (text) {
    const lines = [];
    for (const run of text.runs) {
      const tx = run.tx + text.dx;
      const left = clamp(tx - 1.5, text.bounds[0], text.bounds[2]);
      const right = clamp(tx + (run.layout.width + text.lean * 15) * run.s + 1.5, text.bounds[0], text.bounds[2]);
      for (const y of runMetrics(run.set)) {
        const at = clamp(run.ty + text.dy + y * run.s, text.bounds[1], text.bounds[3]);
        lines.push(`M${num(left)} ${num(at)} H${num(right)}`);
      }
    }
    // Text guides come after the mark guides in data-g order.
    const markCount = mark ? markGuides(mark.candidate, mark.optical).length : 0;
    const w = LINE_PX.guide * unitsPerPx;
    groups.push(`<g class="dln-bp-text">${lines.map((d, i) => `<path class="dln-bp-guide" data-g="${markCount + i}" d="${d}" stroke="currentColor" ${hidden} ${width(w)}/>`).join('')}</g>`);
  }

  if (mark) {
    const { candidate, optical, scale } = mark;
    const local = unitsPerPx / scale;
    const guideW = LINE_PX.guide * local;
    const contourW = Math.min(LINE_PX.contour * local, CONTOUR_MAX[optical]);
    const items = markGuides(candidate, optical).map((g) => `<path class="dln-bp-guide" data-g="${index++}" d="${guidePath(g, mark.bounds)}" stroke="currentColor" ${hidden} ${width(guideW)}/>`);
    candidate.mark[optical].forEach((part, i) => {
      const d = outlinePath(part);
      const caps = part.attrs['stroke-linecap'] ? ` stroke-linecap="${part.attrs['stroke-linecap']}"` : '';
      items.push(`<path class="dln-bp-ink dln-bp-contour" data-c="${i}" d="${d}" stroke="currentColor" stroke-linejoin="round"${caps} ${hidden} ${width(contourW)}/>`);
      if (part.paint === 'fill') {
        const clip = `${idPrefix}-clip-${i}`;
        defs.push(`<clipPath id="${clip}"><path d="${d}"/></clipPath>`);
        items.push(`<path class="dln-bp-ink dln-bp-trace" data-t="${i}" d="${d}" stroke="currentColor" stroke-width="${TRACE_WIDTH[optical].thick}" stroke-linejoin="round" clip-path="url(#${clip})" ${hidden}/>`);
      }
    });
    const transform = mark.transform ? ` transform="${mark.transform}"` : '';
    groups.push(`<g class="dln-bp-mark"${transform}>${items.join('')}</g>`);
  }

  if (text) {
    let letter = 0;
    for (const run of text.runs) {
      const w = (LINE_PX.skeleton * unitsPerPx) / run.s;
      const letters = run.layout.letters
        .map((l) => `<g data-s="${letter++}">${l.strokes.map((d) => `<path d="${d}" ${hidden} ${width(w)}/>`).join('')}</g>`)
        .join('');
      groups.push(`<g class="dln-bp-skeleton dln-bp-${run.cls}" fill="none" stroke="currentColor" ${runStrokeAttrs(run)} transform="${runTransform(text.wm, run, text.lean, text.dx, text.dy)}">${letters}</g>`);
    }
  }

  const defsMarkup = defs.length ? `<defs>${defs.join('')}</defs>` : '';
  return `<g class="dln-bp" aria-hidden="true" fill="none">${defsMarkup}${groups.join('')}</g>`;
}
