import { contrast, hexToOklch, isHex, oklchCss, oklchToHex, type Lch } from "./color";

/**
 * Brand palette: three colour roles chosen by the user. Primary drives
 * buttons, navigation (selected and hover), links, focus and every control
 * (segmented toggles, switches, sliders, checkboxes); secondary drives
 * accents, highlights, glows, the pointer edge light, skeletons and the first
 * chart series; surface tints backgrounds. Status, host and label colours are
 * not part of it and never change with it.
 */
export type PaletteRole = "primary" | "secondary" | "surface";
export type Palette = Record<PaletteRole, string>;
export const PALETTE_ROLES: readonly PaletteRole[] = ["primary", "secondary", "surface"];

export type PalettePresetId = "arcane" | "midnight" | "magenta" | "glacier" | "nebula";
export const PALETTE_PRESETS: readonly { id: PalettePresetId; palette: Palette }[] = [
  { id: "arcane", palette: { primary: "#7f22fe", secondary: "#c4b4ff", surface: "#17161b" } },
  { id: "midnight", palette: { primary: "#4f39f6", secondary: "#00bbfd", surface: "#131721" } },
  { id: "magenta", palette: { primary: "#a600b4", secondary: "#fda5d5", surface: "#1c141b" } },
  { id: "glacier", palette: { primary: "#007491", secondary: "#a4b4ff", surface: "#10191c" } },
  { id: "nebula", palette: { primary: "#9333ea", secondary: "#22d3ee", surface: "#13111c" } },
];

export const DEFAULT_PALETTE: Palette = PALETTE_PRESETS[0]!.palette;

export function matchPreset(palette: Palette): PalettePresetId | null {
  return PALETTE_PRESETS.find((p) => PALETTE_ROLES.every((r) => p.palette[r] === palette[r]))?.id ?? null;
}

const validHex = (c: unknown): string | undefined => (typeof c === "string" && isHex(c) ? c.toLowerCase() : undefined);

/**
 * Keeps valid role values and fills the rest from the default palette. Stored
 * four-role palettes (primary, accent, controls, surface) migrate
 * deterministically: secondary takes the former "Sekundär & Akzent" colour,
 * or the former controls colour when that one is missing or invalid.
 */
export function sanitizePalette(value: unknown, fallback: Palette = DEFAULT_PALETTE): Palette {
  const v = (value && typeof value === "object" ? value : {}) as Record<string, unknown>;
  return {
    primary: validHex(v.primary) ?? fallback.primary,
    secondary: validHex(v.secondary) ?? validHex(v.accent) ?? validHex(v.controls) ?? fallback.secondary,
    surface: validHex(v.surface) ?? fallback.surface,
  };
}

type Theme = "dark" | "light";

const MIN_TEXT_CONTRAST = 4.5;
/** WCAG 1.4.11: controls need 3:1 against their surroundings. */
const MIN_CONTROL_CONTRAST = 3;
const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));

/** Shifts lightness away from the background until the colour reaches the contrast target. */
function readable(start: Lch, against: Lch[], theme: Theme, target = MIN_TEXT_CONTRAST): Lch {
  let color = start;
  for (let i = 0; i < 60 && against.some((bg) => contrast(color, bg) < target); i++) {
    const l = clamp(color.l + (theme === "dark" ? 0.01 : -0.01), 0.05, 0.99);
    color = { ...color, l, c: color.c * (l > 0.9 || l < 0.25 ? 0.92 : 1) };
  }
  return color;
}

/** Light or dark label colour, whichever reads better on the given fill. */
function onFill(fill: Lch): Lch {
  const light = { l: 0.99, c: 0.01, h: fill.h };
  const dark = { l: 0.17, c: 0.02, h: fill.h };
  return contrast(light, fill) >= contrast(dark, fill) ? light : dark;
}

function textStart(base: Lch, theme: Theme): Lch {
  return theme === "dark" ? { l: clamp(base.l + 0.16, 0.7, 0.86), c: base.c * 0.65, h: base.h } : { l: Math.min(base.l, 0.55), c: base.c, h: base.h };
}

/** Text on a tinted fill: near-white or near-black in the fill's hue, whichever reads better. */
function labelOn(fill: Lch): Lch {
  const light = { l: 0.97, c: Math.min(fill.c * 0.3, 0.03), h: fill.h };
  const dark = { l: 0.22, c: Math.min(fill.c * 0.6, 0.05), h: fill.h };
  return contrast(light, fill) >= contrast(dark, fill) ? light : dark;
}

export interface PaletteTokens {
  vars: Record<string, string>;
  /**
   * Contrast ratios shown next to the role inputs. secondaryLabel is the label
   * on the tinted button fill (--secondary), which follows the primary hue.
   */
  report: { primaryLabel: number; primaryText: number; secondaryText: number; secondaryLabel: number; controlLabel: number; controlContrast: number; highlightContrast: number };
  surfaces: { background: string; card: string };
}

/**
 * Derives every palette-driven CSS variable for one colour scheme. The
 * primary fill uses the chosen colour as it is; text variants and labels reach
 * at least 4.5:1, the control fill (switch track, slider range, checked
 * checkbox, segment indicator) at least 3:1 against background and cards. The
 * shadcn names `--secondary*` (tinted button fill) and `--accent` (hover
 * surfaces) follow the primary hue; `--brand-accent*`, `--highlight` and
 * `--skeleton` come from the secondary role. The surface role contributes hue
 * and chroma only, its lightness follows the scheme.
 */
export function paletteTokens(palette: Palette, theme: Theme): PaletteTokens {
  const p = hexToOklch(palette.primary);
  const a = hexToOklch(palette.secondary);
  const s = hexToOklch(palette.surface);
  const tint = Math.min(s.c, 0.048) / 1.2;
  const sh = s.h;
  const dark = theme === "dark";
  const surface = (l: number, c: number): Lch => ({ l, c, h: sh });
  const tinted = (l: number, factor: number, max: number): Lch => ({ l, c: Math.min(p.c * factor, max), h: p.h });

  const background = dark ? surface(0.145, tint) : surface(0.995, tint * 0.5);
  const card = dark ? surface(0.205, tint * 1.2) : surface(1, 0);
  const sidebar = dark ? card : surface(0.985, tint * 0.6);
  const muted = dark ? surface(0.269, tint * 1.2) : surface(0.97, tint * 0.6);
  const code = dark ? surface(0.17, tint * 0.8) : surface(0.975, tint * 0.4);
  const panel = dark ? card : surface(0.985, tint * 0.75);
  const hover = dark ? tinted(0.28, 0.2, 0.04) : tinted(0.955, 0.14, 0.03);
  const sidebarHover = dark ? tinted(0.275, 0.22, 0.045) : tinted(0.945, 0.16, 0.035);
  const secondary = dark ? tinted(0.32, 0.45, 0.09) : tinted(0.92, 0.4, 0.07);
  const secondaryHover = { ...secondary, l: secondary.l + (dark ? 0.05 : -0.04), c: secondary.c * 1.1 };
  const secondaryLabel = labelOn(secondary);
  // Skeletons: muted surface lightness with a quiet secondary tint.
  const skeleton: Lch = { l: muted.l + (dark ? 0.015 : -0.02), c: Math.min(a.c * 0.28, 0.05), h: a.h };

  const textBgs = [background, card, sidebar];
  const primaryText = readable(textStart(p, theme), textBgs, theme);
  const secondaryText = readable(textStart(a, theme), textBgs, theme);
  const control = readable(p, textBgs, theme, MIN_CONTROL_CONTRAST);
  // Glows and the edge light must stay visible on the surfaces, so they get the control target too.
  const highlight = readable(a, textBgs, theme, MIN_CONTROL_CONTRAST);
  const primaryLabel = onFill(p);
  const controlLabel = onFill(control);
  const css = oklchCss;
  const controlFill = control === p ? palette.primary : css(control);
  const highlightFill = highlight === a ? palette.secondary : css(highlight);

  const vars: Record<string, string> = {
    "--background": css(background),
    "--card": css(card),
    "--popover": css(card),
    "--muted": css(muted),
    "--surface": css(panel),
    "--code": css(code),
    "--sidebar": css(sidebar),
    "--primary": palette.primary,
    "--primary-foreground": css(primaryLabel),
    "--primary-text": css(primaryText),
    "--ring": palette.primary,
    "--sidebar-primary": palette.primary,
    "--sidebar-primary-foreground": css(primaryLabel),
    "--sidebar-ring": palette.primary,
    "--chart-2": palette.primary,
    "--brand-accent": palette.secondary,
    "--brand-accent-text": css(secondaryText),
    "--accent": css(hover),
    "--sidebar-accent": css(sidebarHover),
    "--secondary": css(secondary),
    "--secondary-hover": css(secondaryHover),
    "--secondary-foreground": css(secondaryLabel),
    "--chart-1": palette.secondary,
    "--highlight": highlightFill,
    "--skeleton": css(skeleton),
    "--control": controlFill,
    "--control-foreground": css(controlLabel),
    "--control-text": css(primaryText),
  };
  return {
    vars,
    report: {
      primaryLabel: contrast(primaryLabel, p),
      primaryText: Math.min(...textBgs.map((bg) => contrast(primaryText, bg))),
      secondaryText: Math.min(...textBgs.map((bg) => contrast(secondaryText, bg))),
      secondaryLabel: Math.min(contrast(secondaryLabel, secondary), contrast(secondaryLabel, secondaryHover)),
      controlLabel: contrast(controlLabel, control),
      controlContrast: Math.min(...textBgs.map((bg) => contrast(control, bg))),
      highlightContrast: Math.min(...textBgs.map((bg) => contrast(highlight, bg))),
    },
    surfaces: { background: oklchToHex(background), card: oklchToHex(card) },
  };
}

/** Variables owned by the palette; everything else (status colours, borders, foreground) stays in globals.css. */
export const PALETTE_VARS = Object.keys(paletteTokens(DEFAULT_PALETTE, "dark").vars);
