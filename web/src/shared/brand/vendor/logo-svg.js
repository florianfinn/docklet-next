// Pure SVG renderer for the Docklet Next logo candidates. Returns markup
// strings, so it runs in browsers and in Node (static export script).
//
// Colour contract: the mark and "docklet" use the primary colour, "next" the
// secondary colour (tone duo, default). Painted elements carry a class and a
// presentation colour; docklet-logo.css maps the classes to
// --docklet-logo-primary / --docklet-logo-secondary / --docklet-logo-next /
// --docklet-logo-text. Standalone files embed the same hooks as scoped CSS.

import { getCandidate } from './candidates.js';
import { CONSTRUCTION_REF, constructionMarkup } from './construction.js';
import { num } from './glyphs.js';
import { firstLineInk, INITIAL_VARIANTS, lastLineBottom, letteringFor, runGroup, runInk, textBlock } from './lettering.js';
import { MOTION_STYLE_IDS, motionCSS, resolveStyle } from './logo-motion.js';

export { INITIAL_VARIANTS };
export const VARIANTS = ['mark', 'wordmark', 'full', 'compact', 'stacked', ...INITIAL_VARIANTS];
export const TONES = ['duo', 'brand', 'accent', 'mono'];
export const DEFAULT_TONE = 'duo';
export const DEFAULT_CANDIDATE = 'stack';
export const MOTION_STYLES = MOTION_STYLE_IDS;
export const DEFAULT_PRIMARY = '#7F22FE';
/** The user's favourite lockup (2026-10-11): Stack as large D, duo colours, relay entrance. */
export const FAVORITE_LOGO = Object.freeze({ candidate: 'stack', variant: 'dropcap', tone: 'duo', motionStyle: 'relay' });
/** Default secondary ("next" in duo): cyan, one value per background. */
export const DEFAULT_SECONDARY = Object.freeze({ light: '#0092B8', dark: '#00D3F2' });
/** Neutral text colour in standalone files: on light and on dark backgrounds. */
export const NEUTRAL_NEXT = Object.freeze({ light: '#0A0A0A', dark: '#FAFAFA' });

/** Colour source per element group and tone: [mark, docklet, next]; next = --docklet-logo-next, else the secondary. */
export const TONE_COLORS = Object.freeze({
  duo: ['primary', 'primary', 'next'],
  brand: ['primary', 'primary', 'primary'],
  accent: ['primary', 'text', 'primary'],
  mono: ['text', 'text', 'text'],
});

const MARK_UNITS = { regular: 32, small: 16 };
const STACKED_MARK_SCALE = 1.5;
const STACKED_GAP = 7;
const PAD = 0.5;
let sequence = 0;

const escapeXml = (value) => String(value).replace(/[<>&"]/g, (c) => ({ '<': '&lt;', '>': '&gt;', '&': '&amp;', '"': '&quot;' })[c]);

function partElement(part, animatable, color) {
  const paint = part.paint === 'fill' ? `class="dln-p dln-fill" fill="${color}"` : `class="dln-p dln-stroke" fill="none" stroke="${color}"`;
  const attrs = Object.entries(part.attrs)
    .map(([key, value]) => `${key}="${typeof value === 'number' ? num(value) : value}"`)
    .join(' ');
  const length = animatable && part.draw ? ' pathLength="1"' : '';
  const element = `<${part.tag} ${paint} data-part="${part.part}" ${attrs}${length}/>`;
  return part.group ? `<g data-part="${part.group}">${element}</g>` : element;
}

const markTransform = (scale, dx, dy) => (scale !== 1 || dx || dy ? `translate(${num(dx)} ${num(dy)}) scale(${num(scale)})` : '');

function markGroup(candidate, optical, { animatable, color, scale = 1, dx = 0, dy = 0 }) {
  const body = candidate.mark[optical].map((part) => partElement(part, animatable, color)).join('');
  const transform = markTransform(scale, dx, dy);
  return `<g class="dln-mark"${transform ? ` transform="${transform}"` : ''}>${body}</g>`;
}

const union = (boxes) => boxes.reduce((a, b) => [Math.min(a[0], b[0]), Math.min(a[1], b[1]), Math.max(a[2], b[2]), Math.max(a[3], b[3])]);

/** Variant a candidate can render; the initial variants need a letter-shaped mark. */
export function resolveVariant(candidate, variant) {
  if (!VARIANTS.includes(variant)) return 'full';
  return INITIAL_VARIANTS.includes(variant) && !candidate.initial ? 'full' : variant;
}

const paintedLeft = (run) => run.tx - (run.weight / 2) * run.s;
const alignLeft = (run, left) => ({ ...run, tx: run.tx + left - paintedLeft(run) });

/**
 * The mark replaces the first letter. `initial`: the mark takes that letter's
 * painted cap box, "next" starts under it. `dropcap`: the mark spans from the
 * cap top of the first line to the baseline of the last, both lines follow it.
 */
function placeInitial(candidate, variant, block) {
  const ink = candidate.mark.ink.regular;
  const line = firstLineInk(block);
  const bottom = variant === 'initial' ? line.bottom : lastLineBottom(block);
  const scale = (bottom - line.top) / (ink[3] - ink[1]);
  const dx = line.left - ink[0] * scale;
  const dy = line.top - ink[1] * scale;
  const box = [ink[0] * scale + dx, ink[1] * scale + dy, ink[2] * scale + dx, ink[3] * scale + dy];
  const textLeft = box[2] + candidate.initial.gap[variant];
  const [main, next] = block.runs;
  const runs = [alignLeft(main, textLeft), alignLeft(next, variant === 'initial' ? box[0] : textLeft)];
  return { scale, dx, dy, box, runs };
}
/** Builds the markup, viewBox and construction inputs for one variant. */
function compose(candidate, variant, optical, { animatable, colors }) {
  if (variant === 'mark') {
    const size = MARK_UNITS[optical];
    return {
      viewBox: [0, 0, size, size],
      body: markGroup(candidate, optical, { animatable, color: colors.mark }),
      mark: { candidate, optical, scale: 1, dx: 0, dy: 0, transform: '' },
      text: null,
    };
  }
  const wm = letteringFor(candidate, variant);
  const block = textBlock(wm, variant === 'stacked' ? 'center' : 'start');
  const boxes = [];
  const parts = [];
  let mark = null;
  let dx = 0;
  let dy = 0;

  if (INITIAL_VARIANTS.includes(variant)) {
    const placed = placeInitial(candidate, variant, block);
    const { scale: k, dx: mx, dy: my } = placed;
    parts.push(markGroup(candidate, 'regular', { animatable, color: colors.mark, scale: k, dx: mx, dy: my }));
    boxes.push(placed.box);
    mark = { candidate, optical: 'regular', scale: k, dx: mx, dy: my, transform: markTransform(k, mx, my) };
    block.runs = placed.runs;
  } else if (variant === 'full' || variant === 'compact') {
    const ink = candidate.mark.ink.regular;
    parts.push(markGroup(candidate, 'regular', { animatable, color: colors.mark }));
    boxes.push(ink);
    mark = { candidate, optical: 'regular', scale: 1, dx: 0, dy: 0, transform: '' };
    dx = ink[2] + wm.gap;
    dy = wm.align.baseline != null ? wm.align.baseline - block.height : wm.align.center - block.height / 2;
  } else if (variant === 'stacked') {
    const k = STACKED_MARK_SCALE;
    const ink = candidate.mark.ink.regular.map((v) => v * k);
    parts.push(markGroup(candidate, 'regular', { animatable, color: colors.mark, scale: k }));
    boxes.push(ink);
    mark = { candidate, optical: 'regular', scale: k, dx: 0, dy: 0, transform: markTransform(k, 0, 0) };
    dx = (ink[0] + ink[2]) / 2 - block.width / 2;
    dy = ink[3] + STACKED_GAP;
  }

  let index = 0;
  for (const run of block.runs) {
    parts.push(runGroup(wm, run, block.lean, dx, dy, index, { animatable, stroke: run.cls === 'main' ? colors.main : colors.next }));
    boxes.push(runInk(run, block.lean, dx, dy));
    index += run.layout.letters.length;
  }
  const box = union(boxes);
  return {
    viewBox: [box[0] - PAD, box[1] - PAD, box[2] - box[0] + 2 * PAD, box[3] - box[1] + 2 * PAD],
    body: parts.join(''),
    mark,
    text: { wm, runs: block.runs, lean: block.lean, dx, dy },
  };
}

/**
 * Scoped colour hooks for standalone files (no external stylesheet). The
 * --dln-c-* names match docklet-logo.css, so motion keyframes can use them.
 */
function standaloneCSS(scope, tone, nextOn) {
  const s = `.${scope}`;
  const scheme = nextOn === 'dark' ? 'dark' : 'light';
  const [markC, mainC, nextC] = TONE_COLORS[tone].map((key) => `var(--dln-${key})`);
  const rules = [
    `${s}{--dln-primary:var(--docklet-logo-primary,currentColor);--dln-text:var(--docklet-logo-text,${NEUTRAL_NEXT[scheme]});--dln-secondary:var(--docklet-logo-secondary,${DEFAULT_SECONDARY[scheme]});--dln-next:var(--docklet-logo-next,var(--dln-secondary));--dln-c-mark:${markC};--dln-c-main:${mainC};--dln-c-next:${nextC};--dln-guide:var(--docklet-logo-guide,var(--dln-c-next))}`,
    `${s} .dln-fill{fill:var(--dln-c-mark)}`,
    `${s} .dln-stroke,${s} .dln-bp-ink{stroke:var(--dln-c-mark)}`,
    `${s} .dln-word-main,${s} .dln-bp-main{stroke:var(--dln-c-main)}`,
    `${s} .dln-word-next,${s} .dln-bp-next{stroke:var(--dln-c-next)}`,
    `${s} .dln-bp-guide{stroke:var(--dln-guide)}`,
  ];
  if (nextOn === 'auto') rules.push(`@media (prefers-color-scheme:dark){${s}{--dln-text:var(--docklet-logo-text,${NEUTRAL_NEXT.dark});--dln-secondary:var(--docklet-logo-secondary,${DEFAULT_SECONDARY.dark})}}`);
  return rules.join('\n');
}

/**
 * Renders one logo as an SVG string.
 *
 * - variant: mark | wordmark | full | compact | stacked | initial | dropcap
 *   (compact = two-line lettering for narrow sidebars; initial / dropcap =
 *   the mark replaces the D, Stack only, others fall back to full)
 * - optical: regular | small (small = 16 px master, mark variant only)
 * - tone: duo (mark + docklet primary, next secondary; default) | brand (all
 *   primary) | accent (docklet neutral, next primary) | mono (all neutral)
 * - motionStyle: assembly | blueprint | layers | dock | relay (blueprint
 *   adds the hidden construction layer; layers, dock and relay are Stack
 *   only and fall back to assembly elsewhere)
 * - decorative: hides the logo from assistive technology
 * - standalone: adds xmlns, <title>, pixel size, a root colour and scoped
 *   colour CSS for files; nextOn = auto (follows prefers-color-scheme) |
 *   dark | light picks the default secondary and neutral colour of that file
 * - animatable: adds pathLength="1" for draw-on animations
 * - animated: { duration, motionStyle } embeds CSS keyframes (implies animatable)
 * - idPrefix: prefix for element ids and the standalone CSS scope
 */
export function renderLogoSVG(options = {}) {
  const {
    candidate: candidateId = DEFAULT_CANDIDATE,
    variant: variantOption = 'full',
    optical = 'regular',
    tone: toneOption = DEFAULT_TONE,
    label = 'Docklet Next',
    decorative = false,
    standalone = false,
    color = standalone ? DEFAULT_PRIMARY : null,
    nextOn = 'auto',
    height,
    animated = null,
    idPrefix = `dln${(sequence += 1)}`,
  } = options;
  const candidate = getCandidate(candidateId);
  const variant = resolveVariant(candidate, variantOption);
  const tone = TONES.includes(toneOption) ? toneOption : DEFAULT_TONE;
  const motionStyle = resolveStyle(candidate.id, animated?.motionStyle ?? options.motionStyle ?? 'assembly');
  const construction = motionStyle === 'blueprint';
  const animatable = Boolean(options.animatable || animated || construction);
  const resolvedOptical = variant === 'mark' ? optical : 'regular';
  const scheme = nextOn === 'dark' ? 'dark' : 'light';
  const fallback = { text: NEUTRAL_NEXT[scheme], next: DEFAULT_SECONDARY[scheme] };
  const paint = (key) => (key === 'primary' || !standalone ? 'currentColor' : fallback[key]);
  const [mark, main, next] = TONE_COLORS[tone].map(paint);
  const composed = compose(candidate, variant, resolvedOptical, { animatable, colors: { mark, main, next } });
  const { viewBox } = composed;
  const [vx, vy, w, h] = viewBox;
  const scope = standalone || animated ? idPrefix : '';

  let overlay = '';
  if (construction) {
    const rootBounds = [vx + 0.2, vy + 0.2, vx + w - 0.2, vy + h - 0.2];
    const markInfo = composed.mark && {
      ...composed.mark,
      bounds: rootBounds.map((v, i) => (v - (i % 2 ? composed.mark.dy : composed.mark.dx)) / composed.mark.scale),
    };
    const textInfo = composed.text && { ...composed.text, bounds: rootBounds };
    overlay = constructionMarkup({ mark: markInfo, text: textInfo, unitsPerPx: h / CONSTRUCTION_REF, idPrefix });
  }

  const pixelHeight = height ?? (variant === 'mark' ? MARK_UNITS[resolvedOptical] : 32);
  const attrs = [
    standalone ? 'xmlns="http://www.w3.org/2000/svg"' : '',
    `viewBox="${viewBox.map(num).join(' ')}"`,
    height || standalone ? `height="${num(pixelHeight)}"` : '',
    height || standalone ? `width="${num((pixelHeight * w) / h)}"` : '',
    `class="dln-logo${scope ? ` ${scope}` : ''}"`,
    `data-candidate="${candidate.id}"`,
    `data-variant="${variant}"`,
    `data-optical="${resolvedOptical}"`,
    `data-tone="${tone}"`,
    motionStyle !== 'assembly' ? `data-motion-style="${motionStyle}"` : '',
    color ? `color="${escapeXml(color)}"` : '',
    decorative ? 'aria-hidden="true" focusable="false"' : `role="img" aria-label="${escapeXml(label)}"`,
  ].filter(Boolean);

  const title = standalone && !decorative ? `<title>${escapeXml(label)}</title>` : '';
  const css = [
    standalone ? standaloneCSS(scope, tone, nextOn) : '',
    animated ? motionCSS(candidate.id, variant, animated.duration, scope, { style: motionStyle, optical: resolvedOptical }) : '',
  ].filter(Boolean);
  const style = css.length ? `<style>${css.join('\n')}</style>` : '';
  return `<svg ${attrs.join(' ')}>${title}${style}${composed.body}${overlay}</svg>`;
}
