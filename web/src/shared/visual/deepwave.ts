// Pure math of the "deepwave" dot background: a perspective point field whose
// rows crowd towards a horizon at one third of the height, moved by three
// overlapping waves. Derived from the sign-in background of the earlier
// docklet hub (same maintainer); every pixel measure scales with the surface
// height, so the field also fits the dashboard summary card.

/** No point above this share of the height; the upper third stays empty. */
export const DEEPWAVE_HORIZON = 0.33;
const ROWS = 30;
/** Pushes the rows towards the horizon; this exponent is the whole perspective. */
const DEPTH_EXPONENT = 1.8;
/** Front-row spacing in px at full scale. */
const GRID_STEP = 28;
const MIN_SPACING = 17;
/** Height in px at which the field has its original measures. */
const FULL_HEIGHT = 900;
/** Upper bound of points per surface; wider rows space out instead of adding points. */
export const DEEPWAVE_MAX_POINTS = 3200;

/**
 * Wave period and depth setting at which the field moves exactly as designed;
 * other settings scale speed and displacement relative to these.
 */
export const DEEPWAVE_REFERENCE = { period: 32, depth: 0.6 } as const;

interface WavePart {
  ax: number;
  av: number;
  len: number;
  period: number;
  weight: number;
}

/** Three waves whose periods are no integer multiples of each other: the sum repeats after 189 s. */
const PARTS: readonly WavePart[] = [
  { ax: 0.55, av: 0.85, len: 620, period: 9, weight: 1 },
  { ax: -0.8, av: 0.35, len: 380, period: 13.5, weight: 0.5 },
  { ax: 0.1, av: 1, len: 1000, period: 21, weight: 0.75 },
];
const TOTAL = PARTS.reduce((s, p) => s + p.weight, 0);
const TAU = Math.PI * 2;

/**
 * Point field in structure-of-arrays form, back rows first (front drawn last).
 * `phase0…2` hold each point's fixed spatial phase per wave part in radians,
 * so a frame only adds the time term.
 */
export interface DeepwaveField {
  count: number;
  x: Float32Array;
  y: Float32Array;
  /** Vertical displacement in px at full lift. */
  amp: Float32Array;
  /** Diameter in px. */
  size: Float32Array;
  /** Base opacity 0 … 1, higher at the front. */
  alpha: Float32Array;
  phase0: Float32Array;
  phase1: Float32Array;
  phase2: Float32Array;
}

export function deepwaveScale(height: number): number {
  return Math.min(1, Math.max(0.3, height / FULL_HEIGHT));
}

/** Row layout before allocation: y, spacing and depth factor per row, plus the point total. */
function rows(width: number, height: number, widen: number) {
  const s = deepwaveScale(height);
  const step = GRID_STEP * s;
  const horizon = height * DEEPWAVE_HORIZON;
  const out: { y: number; spacing: number; df: number; v: number }[] = [];
  let total = 0;
  for (let row = ROWS - 1; row >= 0; row--) {
    const df = Math.pow(1 - row / (ROWS - 1), DEPTH_EXPONENT);
    const y = horizon + (height + step - horizon) * df;
    const spacing = Math.max(step * (0.4 + 0.6 * df), MIN_SPACING * s) * widen;
    out.push({ y, spacing, df, v: row * GRID_STEP * 1.7 });
    total += Math.floor((width + 2 * step) / spacing) + 1;
  }
  return { s, step, out, total };
}

/** Builds the field for a measured surface; at most DEEPWAVE_MAX_POINTS points. */
export function createDeepwaveField(width: number, height: number): DeepwaveField {
  const w = Math.max(0, width);
  const h = Math.max(0, height);
  // Each row adds an endpoint regardless of spacing, so one proportional step can overshoot; widen until within budget.
  let plan = rows(w, h, 1);
  for (let widen = 1; plan.total > DEEPWAVE_MAX_POINTS; ) {
    widen *= Math.max(1.01, plan.total / DEEPWAVE_MAX_POINTS);
    plan = rows(w, h, widen);
  }
  const { s, step, out } = plan;
  const n = w > 0 && h > 0 ? plan.total : 0;
  const f: DeepwaveField = {
    count: 0,
    x: new Float32Array(n),
    y: new Float32Array(n),
    amp: new Float32Array(n),
    size: new Float32Array(n),
    alpha: new Float32Array(n),
    phase0: new Float32Array(n),
    phase1: new Float32Array(n),
    phase2: new Float32Array(n),
  };
  const phases = [f.phase0, f.phase1, f.phase2];
  let i = 0;
  if (n > 0) for (const r of out) {
    const across = s * (0.4 + 0.6 * r.df);
    for (let x = -step; x <= w + step && i < n; x += r.spacing, i++) {
      const u = (x - w / 2) / across;
      f.x[i] = x;
      f.y[i] = r.y;
      f.amp[i] = (14 + 56 * r.df) * s;
      f.size[i] = (2 + 4.2 * r.df) * Math.max(0.6, s);
      f.alpha[i] = 0.16 + 0.46 * r.df;
      PARTS.forEach((p, k) => (phases[k]![i] = (TAU * (p.ax * u + p.av * r.v)) / p.len));
    }
  }
  f.count = i;
  return f;
}

/** Height -1 … 1 of point `i` at `t` seconds of wave time. */
export function deepwaveHeight(f: DeepwaveField, i: number, t: number): number {
  return (
    (PARTS[0]!.weight * Math.sin((f.phase0[i] ?? 0) + (TAU * t) / PARTS[0]!.period) +
      PARTS[1]!.weight * Math.sin((f.phase1[i] ?? 0) + (TAU * t) / PARTS[1]!.period) +
      PARTS[2]!.weight * Math.sin((f.phase2[i] ?? 0) + (TAU * t) / PARTS[2]!.period)) /
    TOTAL
  );
}

/** Wave time for wall-clock seconds: a longer period setting slows the field down. */
export function deepwaveTime(seconds: number, period: number): number {
  return (seconds * DEEPWAVE_REFERENCE.period) / Math.max(1, period);
}

/** Displacement factor of the depth setting; 0 keeps the points on their rows. */
export function deepwaveLift(depth: number): number {
  return Math.max(0, depth) / DEEPWAVE_REFERENCE.depth;
}
