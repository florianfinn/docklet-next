import { DEFAULT_LAYOUT, layoutStore, mergeLayout, type DashboardLayout } from "@/features/dashboard/dashboard-layout";
import { DEFAULT_FLEET_VIEW, fleetViewStore, mergeFleetView, type FleetView } from "@/features/dashboard/fleet-view-store";
import { DEFAULT_HELP, helpStore, mergeHelp, type HelpPrefs } from "@/shared/help/help-store";
import { createStore, useStore } from "@/shared/lib/store";
import { DEFAULT_PREFERENCES, preferencesStore, type Preferences } from "@/shared/preferences/preferences-store";
import { SHIPPED_DESIGN } from "@/shared/preferences/shipped-design";
import { DEFAULT_UI_STYLE, mergeUiStyle, uiStyleStore, type UiStyle } from "@/shared/preferences/ui-style-store";
import { DEFAULT_VISUAL_STYLE, mergeVisualStyle, visualStyleStore, type VisualStyle } from "@/shared/preferences/visual-style-store";

import { DEFAULT_TOKENS, labStore, normalizeTokens, type DesignTokens } from "./design-tokens";

/**
 * Shareable design code: presentation settings only, so a user can send the
 * look they prefer and it can become the shipped default. Never contains
 * hosts, host selection or order, service addresses, tokens, passwords or
 * other operating data. Every section is validated with the store's own
 * merge function before it is written through the store (no raw storage).
 */
export const CODE_PREFIX = "DOCKLET-DESIGN-";
export const CODE_VERSION = 1;
export const MAX_CODE_LENGTH = 24_000;

const OVERVIEW_KEYS = ["stacksMode", "containersMode", "stacksLayout", "containersLayout", "volumesLayout", "imagesLayout", "workspaceAside"] as const;
type OverviewPrefs = Pick<Preferences, (typeof OVERVIEW_KEYS)[number]>;
const OVERVIEW_VALUES: Record<(typeof OVERVIEW_KEYS)[number], readonly string[]> = {
  stacksMode: ["compact", "advanced"],
  containersMode: ["compact", "advanced"],
  stacksLayout: ["table", "hosts"],
  containersLayout: ["table", "hosts"],
  volumesLayout: ["table", "hosts"],
  imagesLayout: ["table", "hosts"],
  workspaceAside: ["off", "details", "split"],
};

export interface DesignSnapshot {
  tokens?: Omit<DesignTokens, "ignoreReducedMotion">;
  uiStyle?: UiStyle;
  visual?: VisualStyle;
  dashboard?: Pick<DashboardLayout, "order" | "hidden" | "sizes">;
  fleet?: Omit<FleetView, "hostOrder">;
  overview?: OverviewPrefs;
  help?: HelpPrefs;
}

export const SECTIONS = ["tokens", "uiStyle", "visual", "dashboard", "fleet", "overview", "help"] as const;
export type SectionId = (typeof SECTIONS)[number];

/** Text that looks like an address, a path or a secret never enters a code. */
const SAFE_STRING = /^[\w #().,%+-]{0,40}$/;

/** Keeps only the keys of `shape` whose values have the same primitive type; drops unsafe strings. */
function sameShape<T extends object>(value: unknown, shape: T): Partial<T> {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  const out: Record<string, unknown> = {};
  for (const [key, def] of Object.entries(shape)) {
    const v = (value as Record<string, unknown>)[key];
    if (typeof v !== typeof def || v === null) continue;
    if (typeof v === "string" && !SAFE_STRING.test(v)) continue;
    if (typeof v === "number" && !Number.isFinite(v)) continue;
    if (Array.isArray(def)) {
      if (Array.isArray(v)) out[key] = v.filter((x) => typeof x === "boolean" || (typeof x === "number" && Number.isFinite(x)) || (typeof x === "string" && SAFE_STRING.test(x))).slice(0, 64);
    } else if (typeof v === "object") out[key] = sameShape(v, def);
    else out[key] = v;
  }
  return out as Partial<T>;
}

function cleanTokens(raw: unknown): DesignSnapshot["tokens"] {
  const { ignoreReducedMotion: _ignored, ...tokens } = normalizeTokens(sameShape(raw, DEFAULT_TOKENS));
  return tokens;
}

function cleanUiStyle(raw: unknown): UiStyle {
  return mergeUiStyle(sameShape(raw, DEFAULT_UI_STYLE), DEFAULT_UI_STYLE);
}

function cleanDashboard(raw: unknown): DesignSnapshot["dashboard"] {
  const r = (raw && typeof raw === "object" ? raw : {}) as Record<string, unknown>;
  const merged = mergeLayout({ order: Array.isArray(r.order) ? r.order : DEFAULT_LAYOUT.order, hidden: Array.isArray(r.hidden) ? r.hidden : DEFAULT_LAYOUT.hidden, sizes: r.sizes && typeof r.sizes === "object" ? r.sizes : {}, services: DEFAULT_LAYOUT.services }, DEFAULT_LAYOUT);
  return { order: merged.order, hidden: merged.hidden, sizes: merged.sizes };
}

function cleanFleet(raw: unknown): DesignSnapshot["fleet"] {
  const { hostOrder: _hosts, ...fleet } = mergeFleetView({ ...sameShape(raw, DEFAULT_FLEET_VIEW), hostOrder: [] }, DEFAULT_FLEET_VIEW);
  return fleet;
}

function cleanOverview(raw: unknown): OverviewPrefs {
  const r = (raw && typeof raw === "object" ? raw : {}) as Record<string, unknown>;
  return Object.fromEntries(OVERVIEW_KEYS.map((k) => [k, OVERVIEW_VALUES[k].includes(r[k] as string) ? r[k] : DEFAULT_PREFERENCES[k]])) as OverviewPrefs;
}

const CLEAN: Record<SectionId, (raw: unknown) => unknown> = {
  tokens: cleanTokens,
  uiStyle: cleanUiStyle,
  visual: (raw) => mergeVisualStyle(raw, DEFAULT_VISUAL_STYLE),
  dashboard: cleanDashboard,
  fleet: cleanFleet,
  overview: cleanOverview,
  help: (raw) => mergeHelp(raw, DEFAULT_HELP),
};

/** Validates every known section; unknown sections are dropped. */
export function sanitizeSnapshot(raw: unknown): DesignSnapshot {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return {};
  const out: Record<string, unknown> = {};
  for (const id of SECTIONS) {
    const value = (raw as Record<string, unknown>)[id];
    if (value && typeof value === "object") out[id] = CLEAN[id](value);
  }
  return out as DesignSnapshot;
}

/** Current presentation settings of this browser. */
export function captureSnapshot(): DesignSnapshot {
  const prefs = preferencesStore.get();
  return sanitizeSnapshot({
    tokens: labStore.get().tokens,
    uiStyle: uiStyleStore.get(),
    visual: visualStyleStore.get(),
    dashboard: layoutStore.get(),
    fleet: fleetViewStore.get(),
    overview: Object.fromEntries(OVERVIEW_KEYS.map((k) => [k, prefs[k]])),
    help: helpStore.get(),
  });
}

/** Shipped default of every section; the stores derive their defaults from the same data. */
export function defaultSnapshot(): DesignSnapshot {
  return sanitizeSnapshot({ tokens: DEFAULT_TOKENS, uiStyle: DEFAULT_UI_STYLE, visual: DEFAULT_VISUAL_STYLE, dashboard: DEFAULT_LAYOUT, fleet: DEFAULT_FLEET_VIEW, overview: SHIPPED_DESIGN.overview, help: DEFAULT_HELP });
}

function toBase64Url(text: string): string {
  let binary = "";
  for (const byte of new TextEncoder().encode(text)) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function fromBase64Url(text: string): string {
  const b64 = text.replace(/-/g, "+").replace(/_/g, "/");
  const binary = atob(b64 + "=".repeat((4 - (b64.length % 4)) % 4));
  return new TextDecoder("utf-8", { fatal: true }).decode(Uint8Array.from(binary, (c) => c.charCodeAt(0)));
}

export function encodeDesignCode(snapshot: DesignSnapshot): string {
  return `${CODE_PREFIX}${CODE_VERSION}:${toBase64Url(JSON.stringify({ schema: "docklet-design", sections: sanitizeSnapshot(snapshot) }))}`;
}

/** Code of the shipped default design, shown in the lab and usable as a starting point. */
export const SHIPPED_DEFAULT_CODE: string = encodeDesignCode(defaultSnapshot());

export type DecodeError = "empty" | "prefix" | "version" | "size" | "decode" | "schema";
export type DecodeResult = { ok: true; snapshot: DesignSnapshot; sections: SectionId[] } | { ok: false; error: DecodeError };

export function decodeDesignCode(input: string): DecodeResult {
  const code = input.replace(/\s+/g, "");
  if (!code) return { ok: false, error: "empty" };
  if (code.length > MAX_CODE_LENGTH) return { ok: false, error: "size" };
  const head = /^DOCKLET-DESIGN-(\d+):([\w-]+)$/.exec(code);
  if (!head) return { ok: false, error: code.startsWith(CODE_PREFIX) ? "decode" : "prefix" };
  if (Number(head[1]) !== CODE_VERSION) return { ok: false, error: "version" };
  let parsed: unknown;
  try {
    parsed = JSON.parse(fromBase64Url(head[2] ?? ""));
  } catch {
    return { ok: false, error: "decode" };
  }
  const body = parsed as { schema?: unknown; sections?: unknown } | null;
  if (!body || body.schema !== "docklet-design") return { ok: false, error: "schema" };
  const snapshot = sanitizeSnapshot(body.sections);
  const sections = SECTIONS.filter((id) => id in snapshot);
  return sections.length ? { ok: true, snapshot, sections } : { ok: false, error: "schema" };
}

/**
 * Writes a validated snapshot through the stores. Sections missing from the
 * snapshot stay as they are; host order, host selection, widget service
 * connections and the lab's reduced-motion override are always kept.
 */
export function applySnapshot(raw: DesignSnapshot): SectionId[] {
  const s = sanitizeSnapshot(raw);
  if (s.tokens) {
    const tokens = s.tokens;
    labStore.set((lab) => ({ ...lab, tokens: { ...tokens, ignoreReducedMotion: lab.tokens.ignoreReducedMotion }, active: null }));
  }
  if (s.uiStyle) uiStyleStore.set(s.uiStyle);
  if (s.visual) visualStyleStore.set(s.visual);
  if (s.dashboard) {
    const dashboard = s.dashboard;
    layoutStore.set((l) => mergeLayout({ ...dashboard, services: l.services }, DEFAULT_LAYOUT));
  }
  if (s.fleet) {
    const fleet = s.fleet;
    fleetViewStore.set((f) => mergeFleetView({ ...fleet, hostOrder: f.hostOrder }, DEFAULT_FLEET_VIEW));
  }
  if (s.overview) {
    const overview = s.overview;
    preferencesStore.set((p) => ({ ...p, ...overview }));
  }
  if (s.help) helpStore.set(s.help);
  return SECTIONS.filter((id) => id in s);
}

export interface Favorite {
  code: string;
  savedAt: string;
}

/** Only a code that still decodes survives a reload; anything else is dropped. */
export function mergeFavorite(stored: unknown): Favorite | null {
  const f = stored as Partial<Favorite> | null;
  if (!f || typeof f.code !== "string" || typeof f.savedAt !== "string") return null;
  return decodeDesignCode(f.code).ok && !Number.isNaN(Date.parse(f.savedAt)) ? { code: f.code, savedAt: f.savedAt } : null;
}

export const favoriteStore = createStore<Favorite | null>(null, { key: "docklet-react-opus.design-favorite", version: 1, merge: mergeFavorite });

export function useFavorite(): Favorite | null {
  return useStore(favoriteStore, (f) => f);
}

export function saveFavorite(now: Date = new Date()): Favorite {
  const favorite = { code: encodeDesignCode(captureSnapshot()), savedAt: now.toISOString() };
  favoriteStore.set(favorite);
  return favorite;
}

export function clearFavorite(): void {
  favoriteStore.set(null);
}

/** Every section back to the shipped default; host order, selection and widget connections stay. */
export function resetToShipped(): "shipped" {
  applySnapshot(defaultSnapshot());
  return "shipped";
}

/** The local favourite (sections it lacks from the shipped default), else the shipped default. */
export function resetToFavorite(): "favorite" | "shipped" {
  const local = favoriteStore.get();
  const fromLocal = local ? decodeDesignCode(local.code) : null;
  if (!fromLocal?.ok) return resetToShipped();
  applySnapshot({ ...defaultSnapshot(), ...fromLocal.snapshot });
  return "favorite";
}
