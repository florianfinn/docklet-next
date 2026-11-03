// Source: docklet-next/docs/design/preferences.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original profile preference document contract for issue #339.

/** Sections of the per-user preference document; each replaces as a whole. */
export const UI_SECTIONS = ["appearance", "uiStyle", "visualStyle", "views", "hidden", "help", "dashboard", "fleetView", "favorite", "sidebar"] as const;
export type UiSection = (typeof UI_SECTIONS)[number];

export const UI_SECTION_LIMIT = 16 * 1024;
export const UI_DOCUMENT_LIMIT = 64 * 1024;

export interface UiPreferencesDocument {
  revision: number;
  /** Raw section values; consumers decode them before use. */
  sections: Partial<Record<UiSection, unknown>>;
}

export interface UiPreferencesApi {
  /** The signed-in user's document, or the hub document without login. */
  getUiPreferences(): Promise<UiPreferencesDocument>;
  /** Replaces one section; the value must be a JSON object. */
  putUiPreference(section: UiSection, value: object): Promise<UiPreferencesDocument>;
}

export function isUiSection(value: unknown): value is UiSection {
  return (UI_SECTIONS as readonly unknown[]).includes(value);
}
