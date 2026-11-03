import { createStore, useStore } from "@/shared/lib/store";

import { SHIPPED_DESIGN } from "./shipped-design";

export type OverviewMode = "compact" | "advanced";
/** One table for all hosts, or one block per host side by side. */
export type OverviewLayout = "table" | "hosts";
/** Wide file workspace: nothing beside the editor, a details panel or a second file. */
export type WorkspaceAside = "off" | "details" | "split";

export interface Preferences {
  /** Sidebar host filter; empty means all hosts. */
  hostSelection: string[];
  stacksMode: OverviewMode;
  containersMode: OverviewMode;
  stacksLayout: OverviewLayout;
  containersLayout: OverviewLayout;
  volumesLayout: OverviewLayout;
  imagesLayout: OverviewLayout;
  workspaceAside: WorkspaceAside;
  sidebarHostsOpen: boolean;
  showUnraidRing: boolean;
}

/** Overview modes come from the shipped design; host selection and the sidebar state are per browser only. */
export const DEFAULT_PREFERENCES: Preferences = {
  hostSelection: [],
  ...SHIPPED_DESIGN.overview,
  sidebarHostsOpen: true,
  showUnraidRing: false,
};

const CHOICES = {
  stacksMode: ["compact", "advanced"],
  containersMode: ["compact", "advanced"],
  stacksLayout: ["table", "hosts"],
  containersLayout: ["table", "hosts"],
  volumesLayout: ["table", "hosts"],
  imagesLayout: ["table", "hosts"],
  workspaceAside: ["off", "details", "split"],
} as const satisfies Partial<Record<keyof Preferences, readonly string[]>>;

/** Total decoder for stored and synced values: wrong types fall back field by field. */
export function decodePreferences(stored: unknown, defaults: Preferences = DEFAULT_PREFERENCES): Preferences {
  const s = (stored && typeof stored === "object" && !Array.isArray(stored) ? stored : {}) as Partial<Record<keyof Preferences, unknown>>;
  const choice = <K extends keyof typeof CHOICES>(key: K): Preferences[K] => ((CHOICES[key] as readonly unknown[]).includes(s[key]) ? s[key] : defaults[key]) as Preferences[K];
  const flag = (key: "sidebarHostsOpen" | "showUnraidRing") => (typeof s[key] === "boolean" ? (s[key] as boolean) : defaults[key]);
  return {
    hostSelection: Array.isArray(s.hostSelection) ? [...new Set(s.hostSelection.filter((x): x is string => typeof x === "string" && x.length > 0))] : defaults.hostSelection,
    stacksMode: choice("stacksMode"),
    containersMode: choice("containersMode"),
    stacksLayout: choice("stacksLayout"),
    containersLayout: choice("containersLayout"),
    volumesLayout: choice("volumesLayout"),
    imagesLayout: choice("imagesLayout"),
    workspaceAside: choice("workspaceAside"),
    sidebarHostsOpen: flag("sidebarHostsOpen"),
    showUnraidRing: flag("showUnraidRing"),
  };
}

export const preferencesStore = createStore<Preferences>(DEFAULT_PREFERENCES, {
  key: "docklet-react-opus.preferences",
  version: 1,
  merge: decodePreferences,
});

export function usePreference<K extends keyof Preferences>(key: K): Preferences[K] {
  return useStore(preferencesStore, (p) => p[key]);
}

export function setPreference<K extends keyof Preferences>(key: K, value: Preferences[K]): void {
  preferencesStore.set((p) => ({ ...p, [key]: value }));
}

export function useHostSelection(): string[] {
  return usePreference("hostSelection");
}
