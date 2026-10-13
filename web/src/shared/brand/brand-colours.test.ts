import { describe, expect, it } from "vitest";

import { contrast, hexToOklch, type Lch } from "@/shared/theme/color";
import { PALETTE_PRESETS, paletteTokens } from "@/shared/theme/palette";

const files = import.meta.glob<string>(["/src/shared/brand/brand-logo.css", "/src/styles/visual-effects.css"], { query: "?raw", import: "default", eager: true });
const brandCss = files["/src/shared/brand/brand-logo.css"] ?? "";
const effects = files["/src/styles/visual-effects.css"] ?? "";

/** Parses "#rrggbb" or "oklch(l c h)" as written by paletteTokens. */
function lch(value: string): Lch {
  if (value.startsWith("#")) return hexToOklch(value);
  const [l, c, h] = /oklch\(([\d.]+) ([\d.]+) ([\d.]+)\)/.exec(value)!.slice(1).map(Number) as [number, number, number];
  return { l, c, h };
}

describe("logo colours", () => {
  it("use the saturated palette roles, not the lightened text variants", () => {
    expect(/--docklet-logo-primary: var\(--control\);/.test(brandCss) && /--docklet-logo-secondary: var\(--highlight\);/.test(brandCss)).toBe(true);
    expect(/--docklet-logo-(primary|secondary): var\(--(primary-text|brand-accent-text)\)/.test(brandCss)).toBe(false);
  });

  it("stay the chosen colours where they already read, keep at least 3:1 and are more saturated than the text variants", () => {
    for (const { id, palette } of PALETTE_PRESETS) {
      for (const theme of ["dark", "light"] as const) {
        const { vars, report } = paletteTokens(palette, theme);
        const surfaces = ["--background", "--card", "--sidebar"].map((v) => lch(vars[v]!));
        for (const [role, logo, text, ratio] of [
          ["primary", "--control", "--primary-text", report.controlContrast],
          ["secondary", "--highlight", "--brand-accent-text", report.highlightContrast],
        ] as const) {
          const raw = hexToOklch(palette[role]);
          const colour = lch(vars[logo]!);
          const label = `${id}/${theme}/${role}`;
          expect(ratio >= 3, label).toBe(true);
          if (surfaces.every((s) => contrast(raw, s) >= 3)) expect(vars[logo], label).toBe(palette[role]);
          expect(colour.c >= lch(vars[text]!).c - 0.002, label).toBe(true);
        }
      }
    }
  });
});

describe("dotted background", () => {
  it("keeps a static CSS fallback that the canvas replaces once it runs", () => {
    const before = /\.dot-grid:not\(\.topology\)::before \{([^}]*)\}/.exec(effects)?.[1] ?? "";
    // One 18 px lattice, tinted, never animated: the moving surface is the canvas (shared/visual/dot-field.ts).
    expect([/mask-size:\s*18px 18px/.test(before), /animation/.test(before), /dot-drift|--dot-ax/.test(effects)]).toEqual([true, false, false]);
    expect(/\.dot-grid:has\(> \[data-slot="dot-field"\]\[data-ready\]\)::before \{\s*display: none;/.test(effects)).toBe(true);
    // The surface and its content keep their place.
    expect(/\.dot-grid:not\(\.topology\) \{[^}]*(animation|transform)/.test(effects)).toBe(false);
    expect(/\[data-dot-wave="off"\] \.dot-grid:not\(\.topology\)::before \{\s*background-image: none;/.test(effects)).toBe(true);
  });
});
