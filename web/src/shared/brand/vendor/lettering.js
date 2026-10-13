// Wordmark layout. The lettering is split into two runs, "main" (docklet)
// and "next", each with its own glyph set, scale, stroke weight and caps, so
// "next" can take its own colour and be dropped later without relayout.

import { layoutWord, METRICS, num } from './glyphs.js';

export const CAP = METRICS.baseline - METRICS.capTop;

/** Variants in which the mark stands in for the first letter of "docklet". */
export const INITIAL_VARIANTS = ['initial', 'dropcap'];

/**
 * Lettering settings for a variant; compact switches to two lines, the
 * initial variants drop the letter the mark replaces.
 */
export function letteringFor(candidate, variant) {
  const wm = candidate.wordmark;
  if (INITIAL_VARIANTS.includes(variant) && candidate.initial) {
    const { letter, main } = candidate.initial;
    if (`${letter}${main}` !== wm.main) throw new Error(`${candidate.id}: "${letter}" + "${main}" must spell "${wm.main}"`);
    return { ...wm, main, placement: 'below' };
  }
  if (variant !== 'compact' || !wm.compact) return wm;
  return { ...wm, ...wm.compact, placement: 'below', align: { center: wm.compact.center } };
}

/** Painted top, bottom and left edge of the first line of a two-line block (block units). */
export function firstLineInk(block) {
  const run = block.runs[0];
  const half = (run.weight / 2) * run.s;
  return { top: run.ty + METRICS.capTop * run.s - half, bottom: run.ty + METRICS.baseline * run.s + half, left: run.tx - half };
}

/** Painted bottom of the last line (block units). */
export function lastLineBottom(block) {
  const run = block.runs[block.runs.length - 1];
  return run.ty + (METRICS.baseline + run.weight / 2) * run.s;
}

/** Glyph set and stroke ends of one run. */
export function runStyle(wm, cls) {
  const next = cls === 'next';
  return {
    set: next ? (wm.nextSet ?? wm.set) : wm.set,
    cap: next ? (wm.nextLinecap ?? wm.linecap) : wm.linecap,
    join: next ? (wm.nextLinejoin ?? wm.linejoin) : wm.linejoin,
  };
}

/** Grid lines a run is built on, in glyph units (lowercase adds the x-height). */
export function runMetrics(set) {
  return set === 'lowerRound' ? [METRICS.capTop, METRICS.xTop, METRICS.baseline] : [METRICS.capTop, METRICS.baseline];
}

/**
 * Lays out both runs relative to the text block, whose cap box starts at
 * (0, 0). Inline runs share one baseline, so a smaller "next" (for example
 * small caps at x-height) still sits on the line. A second line in lowercase
 * hangs from its x-height so the line gap reads evenly.
 */
export function textBlock(wm, align = 'start') {
  const lean = Math.tan((-(wm.skew ?? 0) * Math.PI) / 180);
  const mainStyle = runStyle(wm, 'main');
  const nextStyle = runStyle(wm, 'next');
  const main = layoutWord(wm.main, mainStyle.set, { tracking: wm.tracking });
  const next = layoutWord(wm.next, nextStyle.set, { tracking: wm.nextTracking ?? wm.tracking });
  const s1 = wm.scale;
  const run = (cls, style, layout, weight, s, tx, ty) => ({ cls, ...style, layout, weight, s, tx, ty, x0: 0, x1: layout.width });
  let runs;
  let width;
  let height;
  if (wm.placement === 'inline') {
    const s2 = wm.nextScale ?? s1;
    const x2 = (main.width + wm.space) * s1;
    width = x2 + next.width * s2;
    height = CAP * s1;
    runs = [run('main', mainStyle, main, wm.weight, s1, 0, -METRICS.capTop * s1), run('next', nextStyle, next, wm.nextWeight, s2, x2, CAP * s1 - METRICS.baseline * s2)];
  } else {
    const s2 = wm.nextScale;
    const w1 = main.width * s1;
    const w2 = next.width * s2;
    const top2 = nextStyle.set === 'lowerRound' ? METRICS.xTop : METRICS.capTop;
    width = Math.max(w1, w2);
    height = CAP * s1 + wm.lineGap + (METRICS.baseline - top2) * s2;
    const offset = (w) => (align === 'center' ? (width - w) / 2 : 0);
    runs = [run('main', mainStyle, main, wm.weight, s1, offset(w1), -METRICS.capTop * s1), run('next', nextStyle, next, wm.nextWeight, s2, offset(w2), CAP * s1 + wm.lineGap - top2 * s2)];
  }
  return { runs, width: width + lean * CAP * wm.scale, height, lean };
}

/** Painted box of a run in root coordinates. */
export function runInk(run, lean, dx, dy) {
  const half = run.weight / 2;
  const tx = run.tx + dx;
  const ty = run.ty + dy;
  return [
    tx + (run.x0 - half) * run.s,
    ty + (METRICS.capTop - half) * run.s,
    tx + (run.x1 + half + lean * CAP) * run.s,
    ty + (METRICS.baseline + half) * run.s,
  ];
}

/** SVG transform of a run (translate, scale, optional slant around the baseline). */
export function runTransform(wm, run, lean, dx, dy) {
  let transform = `translate(${num(run.tx + dx)} ${num(run.ty + dy)}) scale(${num(run.s)})`;
  if (lean) transform += ` translate(0 ${METRICS.baseline}) skewX(${num(wm.skew)}) translate(0 ${-METRICS.baseline})`;
  return transform;
}

/** Stroke attributes shared by a run and its construction skeleton. */
export function runStrokeAttrs(run) {
  const miter = run.join === 'miter' ? ' stroke-miterlimit="2"' : '';
  return `stroke-linecap="${run.cap}" stroke-linejoin="${run.join}"${miter}`;
}

export function runGroup(wm, run, lean, dx, dy, startIndex, { animatable, stroke = 'currentColor' }) {
  const length = animatable ? ' pathLength="1"' : '';
  const letters = run.layout.letters
    .map((letter, i) => `<g class="dln-letter" data-letter="${startIndex + i}">${letter.strokes.map((d) => `<path d="${d}"${length}/>`).join('')}</g>`)
    .join('');
  return `<g class="dln-word dln-word-${run.cls}" fill="none" stroke="${stroke}" stroke-width="${num(run.weight)}" ${runStrokeAttrs(run)} transform="${runTransform(wm, run, lean, dx, dy)}">${letters}</g>`;
}
