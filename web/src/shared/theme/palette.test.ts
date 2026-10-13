import { describe, expect, it } from "vitest";

import { DEFAULT_TOKENS, normalizeTokens } from "@/shared/theme/design-tokens";

import { hexToOklch, normalizeHex, oklchToHex } from "./color";
import { matchPreset, PALETTE_PRESETS, PALETTE_VARS, paletteTokens, sanitizePalette, type Palette } from "./palette";

/** Small deterministic PRNG so the random palettes are the same on every run. */
function prng(seed: number) {
  let s = seed;
  return () => ((s = (s * 1664525 + 1013904223) % 4294967296) / 4294967296);
}

describe("palette tokens", () => {
  it("keeps every preset readable in both colour schemes", () => {
    for (const p of PALETTE_PRESETS) {
      for (const theme of ["dark", "light"] as const) {
        const r = paletteTokens(p.palette, theme).report;
        expect([r.primaryText, r.secondaryText, r.primaryLabel].every((v) => v >= 4.5), `${p.id}/${theme}`).toBe(true);
        expect(r.controlLabel >= 4 && r.controlContrast >= 3 && r.highlightContrast >= 3, `${p.id}/${theme} control`).toBe(true);
      }
    }
  });

  it("derives readable text and visible controls for arbitrary custom colours while fills stay exact", () => {
    const rand = prng(7);
    const hex = () => `#${Math.floor(rand() * 0xffffff).toString(16).padStart(6, "0")}`;
    for (let i = 0; i < 300; i++) {
      const palette: Palette = { primary: hex(), secondary: hex(), surface: hex() };
      for (const theme of ["dark", "light"] as const) {
        const { vars, report } = paletteTokens(palette, theme);
        expect(Math.min(report.primaryText, report.secondaryText) >= 4.5, `${JSON.stringify(palette)}/${theme}`).toBe(true);
        expect(Math.min(report.primaryLabel, report.controlLabel) >= 4 && report.controlContrast >= 3).toBe(true);
        expect(vars["--primary"] === palette.primary && vars["--brand-accent"] === palette.secondary && vars["--chart-1"] === palette.secondary).toBe(true);
        // Controls follow the primary colour, glows the secondary one; both only move when they need more contrast.
        expect(vars["--control"] === palette.primary || report.controlContrast >= 3).toBe(true);
        expect(vars["--highlight"] === palette.secondary || report.highlightContrast >= 3).toBe(true);
        expect(report.highlightContrast >= 3).toBe(true);
      }
    }
  });

  it("never touches status colours, borders or the main text colour", () => {
    const owned = ["--success", "--warning", "--destructive", "--info", "--foreground", "--border", "--muted-foreground", "--orange"];
    expect(PALETTE_VARS.filter((v) => owned.includes(v))).toEqual([]);
  });

  it("follows the surface hue but not its lightness", () => {
    const bright = paletteTokens({ ...PALETTE_PRESETS[0]!.palette, surface: "#d0e8ff" }, "dark");
    const dim = paletteTokens({ ...PALETTE_PRESETS[0]!.palette, surface: "#101820" }, "dark");
    expect(hexToOklch(bright.surfaces.background).l < 0.2 && hexToOklch(dim.surfaces.background).l < 0.2).toBe(true);
  });

  it("parses hex input and recognises presets", () => {
    expect([normalizeHex("7F22FE"), normalizeHex("#abc"), normalizeHex("blau")]).toEqual(["#7f22fe", "#aabbcc", null]);
    expect(oklchToHex(hexToOklch("#4f39f6"))).toBe("#4f39f6");
    expect(matchPreset(PALETTE_PRESETS[2]!.palette)).toBe("magenta");
    expect(matchPreset({ primary: "#9333ea", secondary: "#22d3ee", surface: "#13111c" })).toBe("nebula");
    expect(matchPreset({ ...PALETTE_PRESETS[2]!.palette, secondary: "#00ff00" })).toBe(null);
    expect(sanitizePalette({ primary: "#123456", secondary: "red", surface: 3 })).toEqual({ ...PALETTE_PRESETS[0]!.palette, primary: "#123456" });
  });

  it("migrates stored four-role palettes to three roles deterministically", () => {
    // Former "Sekundär & Akzent" wins; the controls colour is the fallback; unknown keys are dropped.
    expect(sanitizePalette({ primary: "#FF0000", accent: "#4D0046", controls: "#ff00ea", surface: "#3300ff" })).toEqual({ primary: "#ff0000", secondary: "#4d0046", surface: "#3300ff" });
    expect(sanitizePalette({ primary: "#ff0000", accent: "nope", controls: "#ff00ea", surface: "#3300ff" })).toEqual({ primary: "#ff0000", secondary: "#ff00ea", surface: "#3300ff" });
    expect(Object.keys(sanitizePalette({ primary: "#ff0000", controls: "#ff00ea", legacy: "#000000" }))).toEqual(["primary", "secondary", "surface"]);
    // A stored round-4 lab state loads without errors and keeps every valid colour it can.
    const stored = normalizeTokens({ ...DEFAULT_TOKENS, palette: { primary: "#4f39f6", accent: "#00bbfd", controls: "#6260ff", surface: "#131721" } });
    expect(stored.palette).toEqual(PALETTE_PRESETS[1]!.palette);
    expect(matchPreset(stored.palette)).toBe("midnight");
  });

  it("migrates stored tokens from the former accent hue and drops unknown fields", () => {
    const old = normalizeTokens({ theme: "light", accentHue: 255, accentChroma: 1, tint: 0.01, density: "compact", motion: "calm", legacy: true });
    expect(old.theme === "light" && old.density === "compact" && old.motion === "calm").toBe(true);
    expect(Math.round(hexToOklch(old.palette.primary).h / 5) * 5).toBe(255);
    expect(Object.keys(old).sort()).toEqual(Object.keys(DEFAULT_TOKENS).sort());
  });
});
