import { useEffect, useState } from "react";
import { RouterProvider } from "react-router";
import { IntlProvider } from "use-intl";

import { AdapterProvider } from "@/data/AdapterProvider";
import { useHubTimeZone } from "@/shared/hooks/use-hub-time-zone";
import { createAdapter } from "@/runtime";
import { useApplyDesignTokens, usePrefersReducedMotion, useTokens } from "@/shared/theme/use-design-tokens";
import { usePointerGlow } from "@/shared/lib/pointer-glow";
import { useApplyUiStyle, useUiStyle } from "@/shared/preferences/ui-style-store";
import { useApplyVisualStyle, useVisualStyle } from "@/shared/preferences/visual-style-store";
import { TooltipProvider } from "@/shared/ui/tooltip";

import { catalogs } from "./i18n/catalog";
import { useAppLocale } from "./i18n/locale-store";
import { router } from "./router";

function AppContent({ locale }: { locale: ReturnType<typeof useAppLocale> }) {
  const timeZone = useHubTimeZone();
  return <IntlProvider locale={locale} messages={catalogs[locale]} timeZone={timeZone}>
    <TooltipProvider delayDuration={250}><RouterProvider router={router} /></TooltipProvider>
  </IntlProvider>;
}

/** Root providers with the adapter selected by the build. */
export function App() {
  const [adapter] = useState(createAdapter);
  useApplyDesignTokens();
  useApplyUiStyle();
  const tokens = useTokens();
  const reduced = usePrefersReducedMotion();
  useApplyVisualStyle();
  const locale = useAppLocale();
  useEffect(() => {
    document.documentElement.lang = locale;
    document.title = catalogs[locale].locale.ui.documentTitle;
  }, [locale]);
  usePointerGlow(useUiStyle("pointerGlow"), useUiStyle("pointerGlowIntensity"), (reduced && !tokens.ignoreReducedMotion) || tokens.motion === "off", useVisualStyle("glowInside"), useVisualStyle("glowOutside"));
  return (
    <AdapterProvider adapter={adapter}>
      <AppContent locale={locale} />
    </AdapterProvider>
  );
}
