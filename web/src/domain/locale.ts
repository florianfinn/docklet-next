/** Interface languages; German remains the default; English defines catalogue types. */
export const LOCALES = ["de", "en-US"] as const;
export type AppLocale = (typeof LOCALES)[number];
export const DEFAULT_LOCALE: AppLocale = "de";

/** Intl tag per interface language; German output stays exactly as before the language switch. */
export const INTL_LOCALE: Record<AppLocale, string> = { de: "de-DE", "en-US": "en-US" };

export const isLocale = (v: unknown): v is AppLocale => LOCALES.includes(v as AppLocale);

let active: AppLocale = DEFAULT_LOCALE;

/** Language for plain formatters outside React; the app's locale store keeps it in sync. */
export function activeLocale(): AppLocale {
  return active;
}

export function setActiveLocale(locale: AppLocale): void {
  active = locale;
}

/** Number in the active interface language, e.g. 1,5 (de) or 1.5 (en-US). */
export function formatNumber(value: number, options?: Intl.NumberFormatOptions): string {
  return value.toLocaleString(INTL_LOCALE[active], options);
}

/** BCP 47 tag of the active interface language for Intl formatters. */
export function intlTag(): string {
  return INTL_LOCALE[active];
}

/** Number with percent sign: "12,5 %" in German, "12.5%" in American English. */
export function formatPercentNumber(value: number, options?: Intl.NumberFormatOptions): string {
  return `${formatNumber(value, options)}${active === "de" ? " " : ""}%`;
}

/** Appends the percent sign as the language writes it ("30 %" / "30%"); the number stays as given. */
export function withPercent(value: number | string): string {
  return `${value}${active === "de" ? " " : ""}%`;
}
