// Pure motion plans: turn candidate timing data into per-element keyframes.
// Used by the WAAPI player (docklet-logo.js) and by the CSS keyframe export.
//
// Styles: "assembly" (parts move into place, candidate-specific), "blueprint"
// (construction lines, outline, inking, fill; same for all) and the
// candidate-only styles in `candidate.motions` (Stack: layers, dock, relay).

import { getCandidate, LETTER_KINDS } from './candidates.js';
import { letteringFor } from './lettering.js';
import { BLUEPRINT_DURATION, blueprintSteps } from './motion-blueprint.js';
import { letterCount } from './motion-shared.js';

export { letterCount };
export const MOTION_STYLE_IDS = ['assembly', 'blueprint', 'layers', 'dock', 'relay'];
/** Styles every candidate has; the others exist only where the candidate defines them. */
export const SHARED_STYLES = ['assembly', 'blueprint'];
export const DURATION_RANGES = Object.freeze({
  assembly: Object.freeze({ min: 800, max: 1800 }),
  blueprint: Object.freeze({ min: 1200, max: 3000 }),
  layers: Object.freeze({ min: 1000, max: 2200 }),
  dock: Object.freeze({ min: 700, max: 1600 }),
  relay: Object.freeze({ min: 1200, max: 2600 }),
});
/** Range of the assembly style, kept for callers of the first study round. */
export const DURATION_RANGE = DURATION_RANGES.assembly;

export function supportsStyle(candidateId, style) {
  if (SHARED_STYLES.includes(style)) return true;
  return MOTION_STYLE_IDS.includes(style) && Boolean(getCandidate(candidateId).motions?.[style]);
}

/** The style that actually plays: unknown or unsupported styles fall back to assembly. */
export function resolveStyle(candidateId, style) {
  return supportsStyle(candidateId, style) ? style : 'assembly';
}

const motionOf = (candidate, style) => (style === 'assembly' ? candidate.motion : candidate.motions[style]);

export function defaultDuration(candidateId, style = 'assembly') {
  const candidate = getCandidate(candidateId);
  const resolved = resolveStyle(candidateId, style);
  return resolved === 'blueprint' ? (candidate.blueprint?.duration ?? BLUEPRINT_DURATION) : motionOf(candidate, resolved).duration;
}

export function clampDuration(ms, fallback, style = 'assembly') {
  const range = DURATION_RANGES[style] ?? DURATION_RANGES.assembly;
  const value = Number.isFinite(ms) && ms > 0 ? ms : fallback;
  return Math.min(range.max, Math.max(range.min, value));
}

function assemblySteps(candidate, variant, optical, motion) {
  const hasMark = variant !== 'wordmark';
  const hasWord = variant !== 'mark';
  const markScale = (f) => (hasWord ? f : f / motion.markEnd);
  const wordScale = (f) => (hasMark ? f : (f - motion.letters.from) / (1 - motion.letters.from));
  const steps = [];

  if (hasMark) {
    for (const part of motion.parts) {
      steps.push({
        group: part.part,
        selector: `[data-part="${part.part}"]`,
        keyframes: part.frames,
        from: markScale(part.from),
        to: markScale(part.to),
        origin: typeof part.origin === 'object' ? part.origin[optical] : part.origin,
        label: part.label,
      });
    }
  }

  if (hasWord) {
    const { letters } = motion;
    const kind = LETTER_KINDS[letters.kind];
    const n = letterCount(candidate, variant);
    const mainCount = letteringFor(candidate, variant).main.replace(/\s/g, '').length;
    // `runs` gives "docklet" and "next" their own windows; otherwise one window for all letters.
    const groups = letters.runs
      ? [
          { first: 0, count: mainCount, window: letters.runs.main },
          { first: mainCount, count: n - mainCount, window: letters.runs.next },
        ]
      : [{ first: 0, count: n, window: [letters.from, letters.to] }];
    for (const group of groups) {
      if (group.count <= 0) continue;
      const from = wordScale(group.window[0]);
      const window = wordScale(group.window[1]) - from;
      const each = window * letters.span;
      for (let k = 0; k < group.count; k += 1) {
        const i = group.first + k;
        const start = from + (window - each) * (group.count > 1 ? k / (group.count - 1) : 0);
        steps.push({
          group: 'letters',
          selector: `[data-letter="${i}"]${kind.selector}`,
          keyframes: typeof kind.frames === 'function' ? kind.frames(i < mainCount ? 'main' : 'next') : kind.frames,
          from: start,
          to: start + each,
          origin: kind.origin,
          label: i === 0 ? letters.label : null,
        });
      }
    }
  }
  return steps;
}

/**
 * Returns [{ group, selector, keyframes, delay, duration, origin, label }] in ms.
 * Outside blueprint, mark-only and wordmark-only variants stretch their own
 * part of the timeline over the whole duration, so every variant settles at
 * `total`. An unsupported style plays assembly (see resolveStyle).
 */
export function motionPlan(candidateId, variant = 'full', totalMs, { style = 'assembly', optical = 'regular' } = {}) {
  const candidate = getCandidate(candidateId);
  const resolved = resolveStyle(candidateId, style);
  const total = clampDuration(totalMs, defaultDuration(candidateId, resolved), resolved);
  const steps = resolved === 'blueprint' ? blueprintSteps(candidate, variant, optical) : assemblySteps(candidate, variant, optical, motionOf(candidate, resolved));
  return steps.map(({ from, to, ...step }) => ({ ...step, delay: Math.round(from * total), duration: Math.max(1, Math.round((to - from) * total)) }));
}

/** Summary rows for documentation: one row per group of steps. */
export function timingTable(candidateId, variant, totalMs, options = {}) {
  const rows = new Map();
  for (const step of motionPlan(candidateId, variant, totalMs, options)) {
    const row = rows.get(step.group);
    const end = step.delay + step.duration;
    if (!row) rows.set(step.group, { target: step.group, count: 1, start: step.delay, end, label: step.label });
    else {
      row.count += 1;
      row.start = Math.min(row.start, step.delay);
      row.end = Math.max(row.end, end);
      row.label ??= step.label;
    }
  }
  return [...rows.values()].map((r) => ({ ...r, target: r.count > 1 ? `${r.target} ×${r.count}` : r.target }));
}

const kebab = (prop) => prop.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`);

/** CSS @keyframes and rules for a self-contained animated SVG export. */
export function motionCSS(candidateId, variant, totalMs, scope, options = {}) {
  const plan = motionPlan(candidateId, variant, totalMs, options);
  const css = [];
  plan.forEach((step, index) => {
    const name = `${scope}-k${index}`;
    const frames = step.keyframes.map((frame) => {
      const decls = Object.entries(frame)
        .filter(([key]) => key !== 'offset')
        .map(([key, value]) => (key === 'easing' ? `animation-timing-function:${value}` : `${kebab(key)}:${value}`));
      return `${Math.round(frame.offset * 10000) / 100}%{${decls.join(';')}}`;
    });
    css.push(`@keyframes ${name}{${frames.join('')}}`);
    const origin = step.origin ? `transform-box:fill-box;transform-origin:${step.origin};` : '';
    css.push(`.${scope} ${step.selector}{${origin}animation:${name} ${step.duration}ms linear ${step.delay}ms both}`);
  });
  css.push(`@media (prefers-reduced-motion:reduce){.${scope} *{animation:none!important}}`);
  return css.join('\n');
}
