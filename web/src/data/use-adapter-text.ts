import { useMemo } from "react";
import { useLocale } from "use-intl";

import { localizeDemoText } from "@/runtime";

interface AdapterText {
  (text: string): string;
  (text: string | undefined): string | undefined;
}

/** Renders adapter text in the interface language at the UI boundary. */
export function useAdapterText(): AdapterText {
  const locale = useLocale();
  return useMemo(() => ((text: string | undefined) => localizeDemoText(text, locale)) as AdapterText, [locale]);
}
