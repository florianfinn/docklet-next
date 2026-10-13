import { createStore, mergeKnown, useStore } from "@/shared/lib/store";

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
}

/** Overview modes come from the shipped design; host selection and the sidebar state are per browser only. */
export const DEFAULT_PREFERENCES: Preferences = {
  hostSelection: [],
  ...SHIPPED_DESIGN.overview,
  sidebarHostsOpen: true,
};

export const preferencesStore = createStore<Preferences>(DEFAULT_PREFERENCES, {
  key: "docklet-react-opus.preferences",
  version: 1,
  merge: mergeKnown,
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
