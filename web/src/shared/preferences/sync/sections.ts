// Source: docklet-next/docs/design/preferences.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original binding of profile preference sections to browser stores for issue #339.

import { DEFAULT_LAYOUT, layoutStore, mergeLayout } from "@/features/dashboard/dashboard-layout";
import { DEFAULT_FLEET_VIEW, fleetViewStore, mergeFleetView } from "@/features/dashboard/fleet-view-store";
import type { UiSection } from "@/domain/ui-preferences";
import type { Store } from "@/shared/lib/store";
import { favoriteStore, mergeFavorite } from "@/shared/preferences/favorite-code";
import { DEFAULT_HELP, helpStore, mergeHelp } from "@/shared/preferences/help-store";
import { hiddenStore, mergeHidden } from "@/shared/preferences/hidden-store";
import { decodePreferences, preferencesStore } from "@/shared/preferences/preferences-store";
import { decodeSidebar, sidebarStore } from "@/shared/preferences/sidebar-store";
import { DEFAULT_UI_STYLE, mergeUiStyle, uiStyleStore } from "@/shared/preferences/ui-style-store";
import { DEFAULT_VISUAL_STYLE, mergeVisualStyle, visualStyleStore } from "@/shared/preferences/visual-style-store";
import { designStore, normalizeTokens } from "@/shared/theme/design-tokens";

/**
 * One store per section. `fromWire` is total: any value, including `undefined`
 * for "not stored", yields a valid state; fields that stay local (lab slots,
 * development overrides, demo widget data) are kept from `current`.
 */
export interface SectionBinding<T = unknown> {
  section: UiSection;
  store: Store<T>;
  /** localStorage key whose untagged presence marks a pre-M17 value. */
  legacyKey: string;
  toWire(state: T): object;
  fromWire(raw: unknown, current: T): T;
}

const obj = (raw: unknown): Record<string, unknown> => (raw && typeof raw === "object" && !Array.isArray(raw) ? (raw as Record<string, unknown>) : {});

function bind<T>(binding: SectionBinding<T>): SectionBinding {
  return binding as unknown as SectionBinding;
}

export const SECTION_BINDINGS: readonly SectionBinding[] = [
  bind({
    section: "appearance",
    store: designStore,
    legacyKey: "docklet-react-opus.design-lab",
    toWire: (s) => ({ tokens: s.tokens }),
    fromWire: (raw, current) => ({ ...current, tokens: normalizeTokens(obj(raw).tokens) }),
  }),
  bind({
    section: "uiStyle",
    store: uiStyleStore,
    legacyKey: "docklet-react-opus.ui-style",
    toWire: ({ actionPresentationOverride: _local, ...rest }) => rest,
    fromWire: (raw, current) => ({ ...mergeUiStyle(obj(raw), DEFAULT_UI_STYLE), actionPresentationOverride: current.actionPresentationOverride }),
  }),
  bind({ section: "visualStyle", store: visualStyleStore, legacyKey: "docklet-react-opus.visual-style", toWire: (s) => s, fromWire: (raw) => mergeVisualStyle(obj(raw), DEFAULT_VISUAL_STYLE) }),
  bind({ section: "views", store: preferencesStore, legacyKey: "docklet-react-opus.preferences", toWire: (s) => s, fromWire: (raw) => decodePreferences(raw) }),
  bind({ section: "hidden", store: hiddenStore, legacyKey: "docklet-react-opus.hidden", toWire: (s) => s, fromWire: (raw) => mergeHidden(raw) }),
  bind({ section: "help", store: helpStore, legacyKey: "docklet-react-opus.help", toWire: (s) => s, fromWire: (raw) => mergeHelp(obj(raw), DEFAULT_HELP) }),
  bind({
    section: "dashboard",
    store: layoutStore,
    legacyKey: "docklet-react-opus.dashboard",
    toWire: ({ order, hidden, sizes }) => ({ order, hidden, sizes }),
    fromWire: (raw, current) => {
      const { order, hidden, sizes } = obj(raw);
      return { ...mergeLayout(raw === undefined ? DEFAULT_LAYOUT : { order, hidden, sizes }, DEFAULT_LAYOUT), services: current.services };
    },
  }),
  bind({ section: "fleetView", store: fleetViewStore, legacyKey: "docklet-react-opus.fleet-view", toWire: (s) => s, fromWire: (raw) => mergeFleetView(obj(raw), DEFAULT_FLEET_VIEW) }),
  bind({ section: "favorite", store: favoriteStore, legacyKey: "docklet-react-opus.design-favorite", toWire: (s) => ({ value: s }), fromWire: (raw) => mergeFavorite(obj(raw).value) }),
  bind({ section: "sidebar", store: sidebarStore, legacyKey: "docklet-react-opus.sidebar", toWire: (s) => s, fromWire: (raw) => decodeSidebar(raw) }),
];
