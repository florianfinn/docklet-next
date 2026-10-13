import { describe, expect, it } from "vitest";

import { colourMix, latticeCount, MAX_POINTS, pointJitter, projectPoint, surfaceHeight, WAVE_VECTOR, wavePhase } from "./dot-wave";

const centre = { x: 400, y: 150 };
// Lattice points across a dashboard summary of 1600 × 400 px (every fifth point).
const grid = Array.from({ length: 80 }, (_, i) => ({ x: (i % 20) * 80, y: Math.floor(i / 20) * 100 }));
const at = (p: { x: number; y: number }, t: number, depth = 0.6) => projectPoint(p.x, p.y, surfaceHeight(p.x, p.y, t, 32), depth, centre);

describe("dot wave", () => {
  it("bounds the point count for phone, desktop and 21:9 surfaces", () => {
    const sizes: [number, number][] = [
      [390, 844],
      [1600, 420],
      [3440, 1440],
      [7680, 4320],
    ];
    const counts = sizes.map(([w, h]) => {
      const { cols, rows } = latticeCount(w, h);
      return cols * rows;
    });
    expect(counts.every((n) => n > 0 && n <= MAX_POINTS)).toBe(true);
    // Small surfaces keep the 18 px lattice of the static dots.
    expect(latticeCount(390, 300).spacing).toBe(18);
  });

  it("moves each point by its own phase: crests rise while neighbours sink, not one rigid grid", () => {
    const dy = grid.map((p) => at(p, 4).y - at(p, 0).y);
    const ups = dy.filter((d) => d < -0.2).length;
    const downs = dy.filter((d) => d > 0.2).length;
    const mean = dy.reduce((a, b) => a + b, 0) / dy.length;
    const spread = Math.sqrt(dy.reduce((a, d) => a + (d - mean) ** 2, 0) / dy.length);
    expect([ups > 0, downs > 0, spread > 1]).toEqual([true, true, true]);
  });

  it("is a height surface with perspective, not an orbit: vertical lift, size and nearness change together", () => {
    const p = { x: 400, y: 150 };
    const samples = Array.from({ length: 64 }, (_, i) => at(p, i * 0.5, 1));
    const xs = samples.map((s) => s.x);
    const ys = samples.map((s) => s.y);
    // At the centre the spread is zero: the point only rises and sinks (no circular path).
    expect(Math.max(...xs) - Math.min(...xs)).toBeLessThan(1e-9);
    expect(Math.max(...ys) - Math.min(...ys)).toBeGreaterThan(6);
    const crest = samples.reduce((a, b) => (b.y < a.y ? b : a));
    const trough = samples.reduce((a, b) => (b.y > a.y ? b : a));
    expect([crest.r > trough.r, crest.near > trough.near]).toEqual([true, true]);
  });

  it("keeps points flat at depth 0 and the colour mix within 0 … 1", () => {
    expect(grid.every((p) => at(p, 7, 0).y === p.y && at(p, 7, 0).x === p.x)).toBe(true);
    const mixes = grid.flatMap((p) => [0, 5, 11].map((t) => colourMix(surfaceHeight(p.x, p.y, t, 32))));
    expect(mixes.every((m) => m >= 0 && m <= 1) && new Set(mixes.map((m) => m.toFixed(2))).size > 5).toBe(true);
  });

  it("travels in one direction at one speed: the phase drops by the same amount at every point", () => {
    const period = 32;
    const drops = grid.map((p) => wavePhase(p.x, p.y, 0, period) - wavePhase(p.x, p.y, 3, period));
    const expected = ((2 * Math.PI) / period) * 3;
    expect(drops.every((d) => Math.abs(d - expected) < 1e-9)).toBe(true);
    // A custom period keeps the cycle: after one period every height is back.
    for (const custom of [12, 48]) expect(grid.every((p) => Math.abs(surfaceHeight(p.x, p.y, 0, custom) - surfaceHeight(p.x, p.y, custom, custom)) < 1e-9)).toBe(true);
    expect(grid.every((p) => Math.abs(pointJitter(p.x, p.y)) <= 0.18)).toBe(true);
  });

  it("moves the sampled crest along the wave vector, with the colour crest on the height crest", () => {
    const period = 32;
    const len = Math.hypot(WAVE_VECTOR.x, WAVE_VECTOR.y);
    const dir = { x: WAVE_VECTOR.x / len, y: WAVE_VECTOR.y / len };
    // Sample a line along the travel direction; find the crest (max height) and the colour maximum.
    const line = Array.from({ length: 700 }, (_, i) => ({ x: 200 + dir.x * i, y: 200 + dir.y * i, s: i }));
    const crest = (t: number) => {
      const hs = line.map((p) => surfaceHeight(p.x, p.y, t, period));
      const cs = hs.map(colourMix);
      return { height: line[hs.indexOf(Math.max(...hs))]!.s, colour: line[cs.indexOf(Math.max(...cs))]!.s };
    };
    const shifts = [0, 1, 2, 3].map((t) => crest(t));
    expect(shifts.every((c) => Math.abs(c.height - c.colour) <= 1)).toBe(true);
    // Each frame the crest moves forward by speed × dt (ω / |k| px per s), allowing for the per-point jitter.
    const speed = (2 * Math.PI) / period / len;
    const steps = shifts.slice(1).map((c, i) => c.height - shifts[i]!.height);
    expect(steps.every((d) => d > 0 && Math.abs(d - speed) < 25)).toBe(true);
  });
});
