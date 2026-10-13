// Pure math of the dotted background: a point lattice lying on a gently
// moving 3D surface. Each point keeps its lattice position and is displaced
// by the surface height under it; neighbours sit at different phases of one
// travelling wave, never as one rigid grid.

/** Upper bound of points per surface; larger surfaces widen the lattice instead of adding points. */
export const MAX_POINTS = 2600;
export const BASE_SPACING = 18;

/** Lattice spacing in px for a surface, so the point count stays under MAX_POINTS at any size. */
export function latticeSpacing(width: number, height: number): number {
  const area = Math.max(0, width) * Math.max(0, height);
  return Math.max(BASE_SPACING, Math.sqrt(area / MAX_POINTS));
}

export function latticeCount(width: number, height: number): { cols: number; rows: number; spacing: number } {
  const spacing = latticeSpacing(width, height);
  return { cols: Math.max(0, Math.floor(width / spacing)), rows: Math.max(0, Math.floor(height / spacing)), spacing };
}

const TAU = Math.PI * 2;

/**
 * The one travelling direction of the surface: radians per px along x and y
 * (wavelength about 550 px, heading right and slightly down). Height and
 * colour both read `wavePhase`, so crests and colour move together, in one
 * direction, at one speed.
 */
export const WAVE_VECTOR = { x: 0.0105, y: 0.0042 } as const;

/** Small fixed phase offset per lattice point (±0.18 rad), so the field never moves as a rigid grid. */
export function pointJitter(x: number, y: number): number {
  const n = Math.sin(x * 12.9898 + y * 78.233) * 43758.5453;
  return (n - Math.floor(n) - 0.5) * 0.36;
}

/** Shared phase at (x, y) px and t s: one full cycle per `period` seconds everywhere. */
export function wavePhase(x: number, y: number, t: number, period: number): number {
  return x * WAVE_VECTOR.x + y * WAVE_VECTOR.y - (TAU / Math.max(1, period)) * t + pointJitter(x, y);
}

/**
 * Surface height in -1 … 1: a travelling wave with a softer second harmonic
 * on the same phase, so its profile has rounded crests but no second
 * direction or speed.
 */
export function surfaceHeight(x: number, y: number, t: number, period: number): number {
  const p = wavePhase(x, y, t, period);
  return 0.8 * Math.sin(p) + 0.2 * Math.sin(2 * p + 0.9);
}

/** Primary (0) to secondary (1) colour mix, taken from the height: crests carry the secondary colour. */
export function colourMix(height: number): number {
  return Math.min(1, Math.max(0, 0.5 + 0.5 * height));
}

export interface ProjectedPoint {
  x: number;
  y: number;
  /** Dot radius in px. */
  r: number;
  /** 0 … 1, higher on crests (nearer to the viewer). */
  near: number;
}

/**
 * Perspective view of a lattice point on the surface. `depth` 0 … 1 scales
 * the height: a crest rises (moves up on screen), grows and brightens, a
 * trough sinks back; points also spread slightly away from the centre on
 * crests. Displacement stays within about ±7 px vertically at depth 1.
 */
export function projectPoint(x: number, y: number, h: number, depth: number, centre: { x: number; y: number }): ProjectedPoint {
  const lift = h * depth;
  const spread = 1 + lift * 0.012;
  return {
    x: centre.x + (x - centre.x) * spread,
    y: centre.y + (y - centre.y) * spread - lift * 7,
    r: 1.05 * (1 + lift * 0.38),
    near: 0.5 + 0.5 * lift,
  };
}
