import { activeLocale, DEFAULT_LOCALE, isLocale, setActiveLocale, type AppLocale } from "@/domain/locale";
import { createStore, useStore } from "@/shared/lib/store";

export { LOCALES, type AppLocale } from "@/domain/locale";

/** Per-browser interface language; not a hub setting. */
export const localeStore = createStore<AppLocale>(DEFAULT_LOCALE, {
  key: "docklet-react-opus.locale",
  version: 1,
  merge: (stored, fallback) => (isLocale(stored) ? stored : fallback),
});

setActiveLocale(localeStore.get());
localeStore.subscribe(() => setActiveLocale(localeStore.get()));

export function useAppLocale(): AppLocale {
  return useStore(localeStore, (l) => l);
}

export function setAppLocale(locale: AppLocale): void {
  localeStore.set(locale);
}

export { activeLocale as getAppLocale };
