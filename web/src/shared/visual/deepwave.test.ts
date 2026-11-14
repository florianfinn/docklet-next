import { describe, expect, it } from "vitest";

import { createDeepwaveField, DEEPWAVE_HORIZON, DEEPWAVE_MAX_POINTS, DEEPWAVE_REFERENCE, deepwaveHeight, deepwaveLift, deepwaveTime } from "./deepwave";

describe("deepwave field", () => {
  it("keeps every point below the horizon and draws back rows before front rows", () => {
    const f = createDeepwaveField(1100, 330);
    const ys = Array.from(f.y.subarray(0, f.count));
    expect([f.count > 500, Math.min(...ys) >= 330 * DEEPWAVE_HORIZON, ys.every((y, i) => i === 0 || y >= ys[i - 1]!)]).toEqual([true, true, true]);
  });

  it("makes front points larger and more opaque than back points", () => {
    const f = createDeepwaveField(800, 400);
    const last = f.count - 1;
    expect([(f.size[last] ?? 0) > (f.size[0] ?? 0), (f.alpha[last] ?? 0) > (f.alpha[0] ?? 0), (f.amp[last] ?? 0) > (f.amp[0] ?? 0)]).toEqual([true, true, true]);
  });

  it.each([
    [1920, 300],
    [3840, 2160],
    [7680, 4320],
    [10000, 300],
  ])("caps the point count and the allocated arrays at %i × %i", (width, height) => {
    const f = createDeepwaveField(width, height);
    expect([f.count <= DEEPWAVE_MAX_POINTS, f.x.length <= DEEPWAVE_MAX_POINTS, f.phase2.length <= DEEPWAVE_MAX_POINTS, f.count > DEEPWAVE_MAX_POINTS * 0.8]).toEqual([true, true, true, true]);
  });

  it("stays empty without a size", () => {
    expect([createDeepwaveField(0, 300).count, createDeepwaveField(300, 0).count]).toEqual([0, 0]);
  });

  it("keeps heights within -1 … 1 and moves points over time", () => {
    const f = createDeepwaveField(600, 300);
    const at = (t: number) => Array.from({ length: f.count }, (_, i) => deepwaveHeight(f, i, t));
    const now = at(0);
    const later = at(2);
    expect([now.every((h) => h >= -1 && h <= 1), later.some((h, i) => Math.abs(h - now[i]!) > 0.05)]).toEqual([true, true]);
  });

  it("maps the period and depth settings relative to the designed reference", () => {
    const { period, depth } = DEEPWAVE_REFERENCE;
    expect([deepwaveTime(10, period), deepwaveTime(10, period * 2), deepwaveLift(depth), deepwaveLift(0), deepwaveLift(depth * 1.5)]).toEqual([10, 5, 1, 0, 1.5]);
  });
});
