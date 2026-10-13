import { useMemo } from "react";
import { useLocale } from "use-intl";

import { localizeDemoText } from "@/data/demo/demo-text";

interface DemoText {
  (text: string): string;
  (text: string | undefined): string | undefined;
}

/** Translates texts stored by the demo backend into the interface language at display time. */
export function useDemoText(): DemoText {
  const locale = useLocale();
  return useMemo(() => ((text: string | undefined) => localizeDemoText(text, locale)) as DemoText, [locale]);
}
