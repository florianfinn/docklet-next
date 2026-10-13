// German interface language and application labels.
import type { enLocale } from "./en";

export const deLocale: typeof enLocale = {
  switcher: {
    label: "Sprache",
    hint: "Gilt nur für diesen Browser und sofort; keine Hub-Einstellung.",
    names: { de: "Deutsch", enUS: "English (US)" },
  },
  ui: {
    documentTitle: (import.meta.env.DEV ? "Docklet · React-Prototyp" : "Docklet Next"),
    loading: "Lädt",
    newStack: "Neuer Stack",
  },
};
