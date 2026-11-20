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
  const scheme = (selector: string) => {
    const block = new RegExp(`(?:^|\\n)${selector.replace(".", "\\.")} \\{([^}]*)\\}`).exec(brandCss)?.[1] ?? "";
    const value = (name: string) => new RegExp(`--docklet-logo-${name}: (#[0-9a-f]{6});`).exec(block)?.[1];
    return { primary: value("primary"), secondary: value("secondary") };
  };

  it("keeps violet and cyan brand colours independent of the palette", () => {
    expect(scheme(".brand-logo")).toEqual({ primary: "#7f22fe", secondary: "#0092b8" });
    expect(scheme(".dark .brand-logo")).toEqual({ primary: "#8e51ff", secondary: "#00d3f2" });
    expect(/--docklet-logo-(primary|secondary): var\(--/.test(brandCss)).toBe(false);
  });

  it("reads with at least 3:1 on every preset surface in both schemes", () => {
    for (const { id, palette } of PALETTE_PRESETS) {
      for (const theme of ["dark", "light"] as const) {
        const { vars } = paletteTokens(palette, theme);
        const colours = scheme(theme === "dark" ? ".dark .brand-logo" : ".brand-logo");
        for (const surface of ["--background", "--card", "--sidebar"]) {
          for (const [role, hex] of Object.entries(colours)) {
            expect(contrast(hexToOklch(hex!), lch(vars[surface]!)) >= 3, `${id}/${theme}/${surface}/${role}`).toBe(true);
          }
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
