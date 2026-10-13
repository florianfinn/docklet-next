import { createStore, mergeShallow, useStore } from "@/shared/lib/store";
import { SHIPPED_DESIGN } from "@/shared/preferences/shipped-design";

/**
 * How much the extended explanation tooltips (HelpTooltip, HelpTarget) show:
 * a title with explanation and impact, one sentence, or nothing. Status,
 * provenance and other data hovers are not affected.
 */
export type HelpMode = "full" | "compact" | "off";
export const HELP_MODES: readonly HelpMode[] = ["full", "compact", "off"];

/** A personal reading preference, stored per browser like the appearance settings. */
export interface HelpPrefs {
  mode: HelpMode;
}

const FACTORY_HELP: HelpPrefs = { mode: "full" };

export function mergeHelp(stored: unknown, defaults: HelpPrefs): HelpPrefs {
  const s = mergeShallow(stored, defaults);
  return { mode: HELP_MODES.includes(s.mode) ? s.mode : defaults.mode };
}

/** Shipped default from the favourite design; missing fields fall back to the factory value. */
export const DEFAULT_HELP: HelpPrefs = mergeHelp(SHIPPED_DESIGN.help, FACTORY_HELP);

export const helpStore = createStore<HelpPrefs>(DEFAULT_HELP, {
  key: "docklet-react-opus.help",
  version: 1,
  merge: mergeHelp,
});

export function useHelpMode(): HelpMode {
  return useStore(helpStore, (s) => s.mode);
}

export function setHelpMode(mode: HelpMode): void {
  helpStore.set((s) => (s.mode === mode ? s : { ...s, mode }));
}
