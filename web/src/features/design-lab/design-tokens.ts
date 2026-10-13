import { createStore, mergeShallow } from "@/shared/lib/store";
import { oklchToHex } from "@/shared/theme/color";
import { SHIPPED_DESIGN } from "@/shared/preferences/shipped-design";
import { DEFAULT_PALETTE, paletteTokens, sanitizePalette, type Palette } from "@/shared/theme/palette";

export const THEMES = ["dark", "light"] as const;
export const MOTION_LEVELS = ["off", "calm", "normal", "playful"] as const;
export const DENSITIES = ["compact", "comfortable", "spacious"] as const;
export type MotionLevel = (typeof MOTION_LEVELS)[number];
export type Density = (typeof DENSITIES)[number];

/** Slider ranges of the numeric tokens; stored and imported values are clamped to them. */
export const TOKEN_RANGES = {
  semantic: { min: 0.5, max: 1.5, step: 0.05 },
  glow: { min: 0, max: 1.6, step: 0.1 },
  speed: { min: 0.5, max: 2, step: 0.1 },
  radius: { min: 0.25, max: 1.25, step: 0.05 },
  fontScale: { min: 0.88, max: 1.12, step: 0.02 },
} as const;

const pick = <T extends string>(list: readonly T[], v: unknown, fallback: T): T => (list.includes(v as T) ? (v as T) : fallback);
const clampToken = (key: keyof typeof TOKEN_RANGES, v: unknown, fallback: number) =>
  typeof v === "number" && Number.isFinite(v) ? Math.min(TOKEN_RANGES[key].max, Math.max(TOKEN_RANGES[key].min, v)) : fallback;

export interface DesignTokens {
  theme: (typeof THEMES)[number];
  /** Brand colour roles; status colours are independent of it. */
  palette: Palette;
  /** Multiplier for status colour chroma (success, warning, info, …). */
  semantic: number;
  glow: number;
  motion: MotionLevel;
  speed: number;
  /** Lab-only override to inspect motion while the OS asks for reduced motion. */
  ignoreReducedMotion: boolean;
  density: Density;
  radius: number;
  fontScale: number;
}

/** Arcane 2.15.1 reference values (dark theme, violet primary, neutral grey surfaces). */
export const ARCANE_TOKENS: DesignTokens = {
  theme: "dark",
  palette: { ...DEFAULT_PALETTE, surface: "#171717" },
  semantic: 1,
  glow: 0.6,
  motion: "normal",
  speed: 1,
  ignoreReducedMotion: false,
  density: "comfortable",
  radius: 0.875,
  fontScale: 1,
};

export interface Preset {
  id: string;
  tokens: DesignTokens;
}

const ARCANE_PLUS: DesignTokens = { ...ARCANE_TOKENS, palette: DEFAULT_PALETTE, semantic: 1.12, glow: 1, motion: "playful" };

/** Shipped default tokens; fields the shipped design leaves out fall back to Arcane+. */
export const DEFAULT_TOKENS: DesignTokens = normalizeTokens(SHIPPED_DESIGN.tokens, ARCANE_PLUS);

export const PRESETS: Preset[] = [
  { id: "docklet", tokens: DEFAULT_TOKENS },
  { id: "arcanePlus", tokens: ARCANE_PLUS },
  { id: "arcane", tokens: ARCANE_TOKENS },
  { id: "compact", tokens: { ...ARCANE_TOKENS, density: "compact", radius: 0.6, motion: "calm", glow: 0.4, fontScale: 0.96 } },
  { id: "calm", tokens: { ...ARCANE_TOKENS, semantic: 0.75, glow: 0.2, motion: "calm", speed: 1.3 } },
  { id: "vivid", tokens: { ...ARCANE_TOKENS, palette: { primary: "#4f39f6", secondary: "#a4b4ff", surface: "#191829" }, semantic: 1.35, glow: 1.4, motion: "playful" } },
  { id: "light", tokens: { ...ARCANE_TOKENS, theme: "light", glow: 0.3 } },
];

export interface LabState {
  tokens: DesignTokens;
  slotA?: DesignTokens;
  slotB?: DesignTokens;
  active: "A" | "B" | null;
}


/**
 * Known fields of stored or imported tokens: enums must match, numbers are
 * clamped to the slider ranges, anything else falls back to the default.
 * Older stores kept an accent hue, chroma and surface tint instead of a
 * palette; those become the matching palette roles. Four-role palettes are
 * reduced to three roles by sanitizePalette.
 */
export function normalizeTokens(raw: unknown, d: DesignTokens = DEFAULT_TOKENS): DesignTokens {
  const stored = (raw && typeof raw === "object" ? raw : {}) as Record<string, unknown>;
  let palette = sanitizePalette(stored.palette, d.palette);
  if (!stored.palette && typeof stored.accentHue === "number" && Number.isFinite(stored.accentHue)) {
    const h = stored.accentHue;
    const chroma = typeof stored.accentChroma === "number" && Number.isFinite(stored.accentChroma) ? stored.accentChroma : 1;
    const tint = typeof stored.tint === "number" && Number.isFinite(stored.tint) ? stored.tint : 0;
    const primary = oklchToHex({ l: 0.541, c: 0.281 * chroma, h });
    palette = { primary, secondary: oklchToHex({ l: 0.811, c: 0.111, h }), surface: oklchToHex({ l: 0.205, c: tint * 1.2, h }) };
  }
  return {
    theme: pick(THEMES, stored.theme, d.theme),
    palette,
    semantic: clampToken("semantic", stored.semantic, d.semantic),
    glow: clampToken("glow", stored.glow, d.glow),
    motion: pick(MOTION_LEVELS, stored.motion, d.motion),
    speed: clampToken("speed", stored.speed, d.speed),
    ignoreReducedMotion: stored.ignoreReducedMotion === true,
    density: pick(DENSITIES, stored.density, d.density),
    radius: clampToken("radius", stored.radius, d.radius),
    fontScale: clampToken("fontScale", stored.fontScale, d.fontScale),
  };
}

export const labStore = createStore<LabState>(
  { tokens: DEFAULT_TOKENS, active: null },
  {
    key: "docklet-react-opus.design-lab",
    version: 1,
    merge: (stored, defaults) => {
      const s = mergeShallow(stored, defaults);
      return { ...s, tokens: normalizeTokens(s.tokens), slotA: s.slotA && normalizeTokens(s.slotA), slotB: s.slotB && normalizeTokens(s.slotB) };
    },
  },
);

/**
 * Unsaved appearance edits shown live by the settings page. Only the fields a
 * draft changes are listed, so lab edits to other fields stay visible.
 */
export const tokenPreviewStore = createStore<Partial<DesignTokens> | null>(null);

export function setTokens(patch: Partial<DesignTokens>): void {
  labStore.set((s) => ({ ...s, tokens: { ...s.tokens, ...patch }, active: null }));
}

const MOTION_SCALE: Record<MotionLevel, number> = { off: 0, calm: 0.45, normal: 1, playful: 1.6 };
const SPEED_FACTOR: Record<MotionLevel, number> = { off: 0, calm: 1.35, normal: 1, playful: 0.92 };
const SPACING: Record<Density, string> = { compact: "0.225rem", comfortable: "0.25rem", spacious: "0.28rem" };

/** Writes tokens to the document root so every view picks them up. */
export function applyTokens(tokens: DesignTokens, reducedMotion: boolean): void {
  const root = document.documentElement;
  const reduce = reducedMotion && !tokens.ignoreReducedMotion;
  const level: MotionLevel = reduce && tokens.motion !== "off" ? "calm" : tokens.motion;
  root.classList.toggle("dark", tokens.theme === "dark");
  root.dataset.motion = reduce ? (tokens.motion === "off" ? "off" : "reduced") : tokens.motion;
  const vars: Record<string, string> = {
    ...paletteTokens(tokens.palette, tokens.theme).vars,
    "--sem": String(tokens.semantic),
    "--glow": String(tokens.glow),
    "--motion": String(reduce ? 0 : MOTION_SCALE[level]),
    "--speed": String(level === "off" ? 0.001 : tokens.speed * SPEED_FACTOR[level]),
    "--radius": `${tokens.radius}rem`,
    "--spacing": SPACING[tokens.density],
    "--font-scale": String(tokens.fontScale),
  };
  for (const [k, v] of Object.entries(vars)) root.style.setProperty(k, v);
}

export function motionEnabled(tokens: DesignTokens, reducedMotion: boolean): boolean {
  if (tokens.motion === "off") return false;
  return !(reducedMotion && !tokens.ignoreReducedMotion);
}
