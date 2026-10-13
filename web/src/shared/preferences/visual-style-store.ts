import { useEffect } from "react";

import { GLOW_REACH } from "@/shared/lib/pointer-glow";
import { createStore, useStore } from "@/shared/lib/store";

import { SHIPPED_DESIGN } from "./shipped-design";

/** Lockups offered for the sidebar brand; the collapsed sidebar always shows the mark. */
export const LOGO_VARIANTS = ["dropcap", "initial", "full"] as const;
export const LOGO_MOTIONS = ["relay", "assembly", "layers", "dock", "blueprint"] as const;
export type BrandVariant = (typeof LOGO_VARIANTS)[number];
export type BrandMotion = (typeof LOGO_MOTIONS)[number];

/**
 * Brand, edge light and background effects, tuned in the design lab and kept
 * in the browser. Independent of the UI style store; the pointer glow switch
 * and its intensity stay there.
 */
export interface VisualStyle {
  logoVariant: BrandVariant;
  logoMotion: BrandMotion;
  /** Play the logo entrance once per page load; never on route changes. */
  logoEntrance: boolean;
  /** Edge light reach in px inside the border; capped per element so a block's centre stays dark. */
  glowInside: number;
  /** Edge light reach in px outside the border. */
  glowOutside: number;
  /** Diameter of the light spot on the border in px. */
  glowSize: number;
  /** Width of the lit border line in px. */
  glowLine: number;
  /** Gentle wave over dotted backgrounds (dashboard summary, setup, login): colour and point surface. */
  dotWave: boolean;
  /** Primary/secondary colour of the wave, 0 … 1. */
  dotWaveStrength: number;
  /** Seconds per wave cycle. */
  dotWavePeriod: number;
  /** Height of the 3D point surface, 0 … 1; 0 keeps the points flat while the colour still moves. */
  dotDepth: number;
  /** Expanded desktop sidebar width in rem; the collapsed rail and the phone sheet keep their own width. */
  sidebarWidth: number;
}

export const VISUAL_RANGES = {
  glowInside: { min: 8, max: 64, step: 2 },
  glowOutside: { min: 4, max: 48, step: 2 },
  glowSize: { min: 40, max: 180, step: 4 },
  glowLine: { min: 1, max: 3, step: 0.25 },
  dotWaveStrength: { min: 0, max: 1, step: 0.05 },
  dotWavePeriod: { min: 12, max: 60, step: 2 },
  dotDepth: { min: 0, max: 1, step: 0.05 },
  sidebarWidth: { min: 14, max: 22, step: 0.25 },
} as const satisfies Partial<Record<keyof VisualStyle, { min: number; max: number; step: number }>>;

const FACTORY_VISUAL_STYLE: VisualStyle = {
  logoVariant: "dropcap",
  logoMotion: "relay",
  logoEntrance: true,
  glowInside: GLOW_REACH.inside,
  glowOutside: GLOW_REACH.outside,
  glowSize: 104,
  glowLine: 2,
  dotWave: true,
  dotWaveStrength: 0.55,
  dotWavePeriod: 28,
  dotDepth: 0.6,
  sidebarWidth: 17,
};

const pick = <T extends string>(list: readonly T[], v: unknown, fallback: T): T => (list.includes(v as T) ? (v as T) : fallback);
const num = (key: keyof typeof VISUAL_RANGES, v: unknown, fallback: number) => {
  const { min, max } = VISUAL_RANGES[key];
  return typeof v === "number" && Number.isFinite(v) ? Math.min(max, Math.max(min, v)) : fallback;
};
const bool = (v: unknown, fallback: boolean) => (typeof v === "boolean" ? v : fallback);

/**
 * Known keys only, each validated and clamped; anything else falls back to
 * `defaults`. The former `dotDrift` (orbiting dot offsets) has no successor
 * key: `dotDrift: 0` in an older code or store must not turn off the 3D wave.
 */
export function mergeVisualStyle(stored: unknown, defaults: VisualStyle = DEFAULT_VISUAL_STYLE): VisualStyle {
  const s = (stored && typeof stored === "object" ? stored : {}) as Partial<Record<keyof VisualStyle, unknown>>;
  return {
    logoVariant: pick(LOGO_VARIANTS, s.logoVariant, defaults.logoVariant),
    logoMotion: pick(LOGO_MOTIONS, s.logoMotion, defaults.logoMotion),
    logoEntrance: bool(s.logoEntrance, defaults.logoEntrance),
    glowInside: num("glowInside", s.glowInside, defaults.glowInside),
    glowOutside: num("glowOutside", s.glowOutside, defaults.glowOutside),
    glowSize: num("glowSize", s.glowSize, defaults.glowSize),
    glowLine: num("glowLine", s.glowLine, defaults.glowLine),
    dotWave: bool(s.dotWave, defaults.dotWave),
    dotWaveStrength: num("dotWaveStrength", s.dotWaveStrength, defaults.dotWaveStrength),
    dotWavePeriod: num("dotWavePeriod", s.dotWavePeriod, defaults.dotWavePeriod),
    dotDepth: num("dotDepth", s.dotDepth, defaults.dotDepth),
    sidebarWidth: num("sidebarWidth", s.sidebarWidth, defaults.sidebarWidth),
  };
}

/** Shipped default: the favourite design's visual section over the factory values. */
export const DEFAULT_VISUAL_STYLE: VisualStyle = mergeVisualStyle(SHIPPED_DESIGN.visual, FACTORY_VISUAL_STYLE);

export const visualStyleStore = createStore<VisualStyle>(DEFAULT_VISUAL_STYLE, {
  key: "docklet-react-opus.visual-style",
  version: 1,
  merge: mergeVisualStyle,
});

export function useVisualStyle<K extends keyof VisualStyle>(key: K): VisualStyle[K] {
  return useStore(visualStyleStore, (s) => s[key]);
}

export function setVisualStyle(patch: Partial<VisualStyle>): void {
  visualStyleStore.set((s) => mergeVisualStyle({ ...s, ...patch }, s));
}

/** Unsaved sidebar width of the hub appearance settings; wins over the stored width while set. */
export const sidebarWidthPreviewStore = createStore<number | null>(null);

/**
 * CSS width of the expanded desktop sidebar: the setting, clamped to its range
 * and to 40 % of the viewport so narrow desktops keep room for the page.
 */
export function sidebarWidthCss(rem: number): string {
  const { min, max } = VISUAL_RANGES.sidebarWidth;
  const w = Number.isFinite(rem) ? Math.min(max, Math.max(min, rem)) : DEFAULT_VISUAL_STYLE.sidebarWidth;
  return `min(${w}rem, max(${min}rem, 40vw))`;
}

export function useSidebarWidth(): number {
  const stored = useVisualStyle("sidebarWidth");
  const preview = useStore(sidebarWidthPreviewStore, (w) => w);
  return preview ?? stored;
}

/** Writes the light and wave settings to <html> as CSS variables; pointer moves never touch React. */
export function useApplyVisualStyle(): void {
  const size = useVisualStyle("glowSize");
  const line = useVisualStyle("glowLine");
  const wave = useVisualStyle("dotWave");
  const strength = useVisualStyle("dotWaveStrength");
  const period = useVisualStyle("dotWavePeriod");
  useEffect(() => {
    const root = document.documentElement;
    root.style.setProperty("--glow-size", `${size}px`);
    root.style.setProperty("--glow-line", `${line}px`);
    root.style.setProperty("--dot-wave-strength", String(strength));
    root.style.setProperty("--dot-wave-period", `${period}s`);
    root.dataset.dotWave = wave && strength > 0 ? "on" : "off";
  }, [size, line, wave, strength, period]);
}
