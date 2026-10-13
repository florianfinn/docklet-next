// Blueprint entrance plan: thin guide lines draw, a thin outline traces
// every part, a heavier stroke inks the outline from the inside, the fill
// appears, and the construction fades. Letters get a thin skeleton first
// and are then drawn at full weight. Same step format as the assembly plan.

import { guideCount, TRACE_WIDTH } from './construction.js';
import { letterCount } from './motion-shared.js';
import { IN_OUT, OUT } from './shapes.js';

export const BLUEPRINT_DURATION = 1900;

// Windows are fractions of the total duration; `each` is one element's draw time.
const PHASES = {
  guide: { from: 0, to: 0.3, each: 0.2, peak: 0.55, fade: [0.68, 0.94] },
  contour: { from: 0.1, to: 0.5, each: 0.28, peak: 1, fade: [0.8, 0.96] },
  trace: { from: 0.36, to: 0.74, each: 0.24, fade: [0.84, 0.98] },
  fill: { lag: 0.55, each: 0.14 },
  skeleton: { from: 0.16, to: 0.6, each: 0.22, peak: 0.85, fade: [0.84, 0.98] },
  letters: { from: 0.44, to: 0.94, each: 0.26 },
};

export const BLUEPRINT_PHASES = [
  { key: 'guide', label: 'Hilfslinien zeichnen sich' },
  { key: 'contour', label: 'Dünne Kontur folgt jeder Form' },
  { key: 'trace', label: 'Kräftiger Strich zeichnet die Kontur nach' },
  { key: 'fill', label: 'Fläche wird massiv' },
  { key: 'skeleton', label: 'Buchstabenskelett, dünn' },
  { key: 'letters', label: 'Buchstaben im vollen Strich' },
];

const start = (phase, i, n) => phase.from + (phase.to - phase.from - phase.each) * (n > 1 ? i / (n - 1) : 0);
const dash = { strokeDasharray: '1 1' };
const px = (v) => `${Math.round(v * 1000) / 1000}px`;

/** One element's life: draw on, hold, fade with the construction. */
function life(phase, t0, peak, from = {}, to = {}) {
  const [f0, f1] = phase.fade;
  const span = f1 - t0;
  const at = (t) => (t - t0) / span;
  return {
    from: t0,
    to: f1,
    keyframes: [
      { offset: 0, opacity: 0, ...dash, strokeDashoffset: 1, ...from, easing: OUT },
      { offset: at(t0 + phase.each * 0.15), opacity: peak },
      { offset: at(t0 + phase.each), strokeDashoffset: 0, ...to },
      { offset: at(f0), opacity: peak, easing: IN_OUT },
      { offset: 1, opacity: 0, ...dash, strokeDashoffset: 0, ...to },
    ],
  };
}

/**
 * Blueprint steps as fractions; logo-motion scales them to milliseconds.
 * The final parts and letters end at full opacity without transform, so
 * after the last frame the static logo remains.
 */
export function blueprintSteps(candidate, variant, optical) {
  const hasMark = variant !== 'wordmark';
  const hasWord = variant !== 'mark';
  const steps = [];
  const push = (group, selector, plan) => steps.push({ group, selector, ...plan, label: null });

  const guides = guideCount(candidate, variant, optical);
  for (let i = 0; i < guides; i += 1) {
    push('guide', `[data-g="${i}"]`, life(PHASES.guide, start(PHASES.guide, i, guides), PHASES.guide.peak));
  }

  if (hasMark) {
    const parts = candidate.mark[optical];
    const widths = TRACE_WIDTH[optical];
    parts.forEach((part, i) => {
      const n = parts.length;
      push('contour', `[data-c="${i}"]`, life(PHASES.contour, start(PHASES.contour, i, n), PHASES.contour.peak));
      const t0 = start(PHASES.trace, i, n);
      const selector = `.dln-mark .dln-p[data-part="${part.part}"]`;
      if (part.paint === 'fill') {
        push('trace', `[data-t="${i}"]`, life(PHASES.trace, t0, 1, { strokeWidth: px(widths.thin) }, { strokeWidth: px(widths.thick) }));
        const f = t0 + PHASES.trace.each * PHASES.fill.lag;
        push('fill', selector, {
          from: f,
          to: f + PHASES.fill.each,
          keyframes: [
            { offset: 0, opacity: 0, easing: IN_OUT },
            { offset: 1, opacity: 1 },
          ],
        });
      } else {
        // A stroked part inks itself: it traces from thin to its full width.
        push('trace', selector, {
          from: t0,
          to: t0 + PHASES.trace.each,
          keyframes: [
            { offset: 0, opacity: 0, ...dash, strokeDashoffset: 1, strokeWidth: px(widths.thin), easing: OUT },
            { offset: 0.08, opacity: 1 },
            { offset: 1, opacity: 1, ...dash, strokeDashoffset: 0, strokeWidth: px(part.attrs['stroke-width']) },
          ],
        });
      }
    });
  }

  if (hasWord) {
    const n = letterCount(candidate, variant);
    for (let i = 0; i < n; i += 1) {
      push('skeleton', `[data-s="${i}"] path`, life(PHASES.skeleton, start(PHASES.skeleton, i, n), PHASES.skeleton.peak));
    }
    for (let i = 0; i < n; i += 1) {
      const t0 = start(PHASES.letters, i, n);
      push('letters', `[data-letter="${i}"] path`, {
        from: t0,
        to: t0 + PHASES.letters.each,
        keyframes: [
          { offset: 0, opacity: 0, ...dash, strokeDashoffset: 1, easing: OUT },
          { offset: 0.08, opacity: 1 },
          { offset: 1, opacity: 1, ...dash, strokeDashoffset: 0 },
        ],
      });
    }
  }
  // The first step of each phase carries the phase label for the timing table.
  for (const phase of BLUEPRINT_PHASES) {
    const first = steps.find((step) => step.group === phase.key);
    if (first) first.label = phase.label;
  }
  return steps;
}
