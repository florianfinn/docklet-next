// Source: docklet-next/docs/design/preferences.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original persisted sidebar state for issue #339.

import { createStore, useStore } from "@/shared/lib/store";

export interface SidebarPrefs {
  open: boolean;
}

export const DEFAULT_SIDEBAR: SidebarPrefs = { open: true };

export function decodeSidebar(stored: unknown): SidebarPrefs {
  const open = stored && typeof stored === "object" ? (stored as { open?: unknown }).open : undefined;
  return { open: typeof open === "boolean" ? open : DEFAULT_SIDEBAR.open };
}

/** Desktop sidebar expanded or collapsed; the mobile sheet is not persisted. */
export const sidebarStore = createStore<SidebarPrefs>(DEFAULT_SIDEBAR, { key: "docklet-react-opus.sidebar", version: 1, merge: decodeSidebar });

export function useSidebarOpen(): [boolean, (open: boolean) => void] {
  return [useStore(sidebarStore, (s) => s.open), (open) => sidebarStore.set((s) => (s.open === open ? s : { open }))];
}
