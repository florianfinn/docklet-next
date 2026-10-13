import { act, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { edgeGlow, GLOW_REACH, insideReach, setGlowReach } from "@/shared/lib/pointer-glow";

import { DEFAULT_VISUAL_STYLE, mergeVisualStyle, setVisualStyle, useApplyVisualStyle, visualStyleStore } from "./visual-style-store";

afterEach(() => {
  visualStyleStore.reset();
  setGlowReach(GLOW_REACH);
  localStorage.clear();
});

function Apply() {
  useApplyVisualStyle();
  return null;
}

describe("visual style store", () => {
  it("defaults to the shipped favourite: logo, deep inside reach and a tight outside shimmer", () => {
    expect([DEFAULT_VISUAL_STYLE.logoVariant, DEFAULT_VISUAL_STYLE.logoMotion, DEFAULT_VISUAL_STYLE.glowInside, DEFAULT_VISUAL_STYLE.glowOutside, DEFAULT_VISUAL_STYLE.glowSize, DEFAULT_VISUAL_STYLE.glowLine]).toEqual(["dropcap", "relay", 50, 4, 128, 2]);
  });

  it("validates every stored field and drops unknown keys", () => {
    const merged = mergeVisualStyle({ logoVariant: "mark", logoMotion: "spin", glowInside: 500, glowOutside: -3, glowSize: "big", dotWave: 1, dotWavePeriod: 30, secret: "x" });
    expect(merged).toEqual({ ...DEFAULT_VISUAL_STYLE, glowInside: 64, glowOutside: 4, dotWavePeriod: 30 });
    expect(mergeVisualStyle("nonsense")).toEqual(DEFAULT_VISUAL_STYLE);
    setVisualStyle({ glowLine: 99 });
    expect(visualStyleStore.get().glowLine).toBe(3);
  });

  it("writes light and wave settings to <html>; a switched-off wave is marked off", () => {
    render(<Apply />);
    const root = document.documentElement;
    expect([root.style.getPropertyValue("--glow-size"), root.style.getPropertyValue("--dot-wave-period"), root.dataset.dotWave]).toEqual([`${DEFAULT_VISUAL_STYLE.glowSize}px`, `${DEFAULT_VISUAL_STYLE.dotWavePeriod}s`, "on"]);
    act(() => setVisualStyle({ dotWave: false }));
    expect(root.dataset.dotWave).toBe("off");
    // The 3D depth is clamped to 0 … 1; the orbit drift and its <html> marker are gone.
    act(() => setVisualStyle({ dotDepth: 40 }));
    expect([visualStyleStore.get().dotDepth, root.dataset.dotDrift, root.style.getPropertyValue("--dot-drift")]).toEqual([1, undefined, ""]);
  });

  it("keeps the 3D wave on for an older store or code with dotDrift: 0", () => {
    const merged = mergeVisualStyle({ dotWave: true, dotDrift: 0, dotWaveStrength: 0.9 });
    expect([merged.dotWave, merged.dotDepth > 0, "dotDrift" in merged]).toEqual([true, true, false]);
  });
});

describe("edge light reach", () => {
  it("uses separate inside and outside reach and lights earlier than a linear falloff", () => {
    const wide = { inside: 40, outside: 30 };
    const at = (y: number, reach = wide) => edgeGlow(300, y, 600, 400, 14, reach).strength;
    expect([at(400 - 39) > 0, at(400 - 41), at(400 + 29) > 0, at(400 + 31)]).toEqual([true, 0, true, 0]);
    // Half way into the reach the light is clearly stronger than half.
    expect(at(400 - 20) > 0.7).toBe(true);
    expect(at(400 - 20, { inside: 20, outside: 30 })).toBe(0);
  });

  it("caps the inside reach on blocks so their centre stays dark", () => {
    expect([insideReach(600, 400, 40), insideReach(300, 60, 40), insideReach(600, 400, 8)]).toEqual([40, 24, 8]);
    expect(edgeGlow(300, 200, 600, 400, 14, { inside: 64, outside: 48 }).strength).toBe(0);
  });

  it("lights a row through its exact centre line like the pixels above and below, at any chosen reach", () => {
    // Main navigation (28.8 px), settings navigation (36 px) and a 32 px button; the shipped reach (50/4) and a tight one (8/4).
    for (const reach of [
      { inside: 50, outside: 4 },
      { inside: 8, outside: 4 },
    ]) {
      for (const [w, h] of [
        [249, 28.8],
        [240, 36],
        [120, 32],
      ] as const) {
        const x = w / 2;
        const mid = h / 2;
        const [above, centre, below] = [mid - 1, mid, mid + 1].map((y) => edgeGlow(x, y, w, h, 8, reach).strength);
        // The centre differs from its neighbours by no more than one pixel step of the falloff (≤ 0.1), never drops to 0.
        expect([`${w}x${h}/${reach.inside}`, centre! > 0.2, Math.abs(centre! - above!) <= 0.1, Math.abs(centre! - below!) <= 0.1]).toEqual([`${w}x${h}/${reach.inside}`, true, true, true]);
        // Continuous across the whole height: no step anywhere between two pixels.
        const ys = Array.from({ length: Math.floor(h) + 1 }, (_, i) => edgeGlow(x, i, w, h, 8, reach).strength);
        expect(ys.every((v, i) => i === 0 || Math.abs(v - ys[i - 1]!) < 0.2)).toBe(true);
      }
    }
    // Corners keep their arc: the light point stays on the rounded border near a row's end.
    const corner = edgeGlow(3, 3, 249, 28.8, 8, { inside: 50, outside: 4 });
    expect(corner.strength > 0 && Math.hypot(corner.x - 8, corner.y - 8) <= 14.4 + 1e-9).toBe(true);
  });

  it("applies a new reach to the running glow without a restart", () => {
    setGlowReach({ inside: 10, outside: 10 });
    expect(edgeGlow(300, 400 - 20, 600, 400, 14).strength).toBe(0);
    setGlowReach({ inside: 40, outside: 10 });
    expect(edgeGlow(300, 400 - 20, 600, 400, 14).strength > 0).toBe(true);
  });
});
