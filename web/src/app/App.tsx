import { useEffect, useState } from "react";
import { RouterProvider } from "react-router";
import { IntlProvider } from "use-intl";

import { AdapterProvider } from "@/data/AdapterProvider";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { useApplyDesignTokens, usePrefersReducedMotion, useTokens } from "@/features/design-lab/use-design-lab";
import { usePointerGlow } from "@/shared/lib/pointer-glow";
import { useApplyUiStyle, useUiStyle } from "@/shared/preferences/ui-style-store";
import { useApplyVisualStyle, useVisualStyle } from "@/shared/preferences/visual-style-store";
import { TooltipProvider } from "@/shared/ui/tooltip";

import { catalogs } from "./i18n/catalog";
import { useAppLocale } from "./i18n/locale-store";
import { router } from "./router";

/** Root providers. Swap createDemoAdapter for an HTTP adapter to leave the demo. */
export function App() {
  const [adapter] = useState(createDemoAdapter);
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
    <IntlProvider locale={locale} messages={catalogs[locale]} timeZone="Europe/Berlin">
      <AdapterProvider adapter={adapter}>
        <TooltipProvider delayDuration={250}>
          <RouterProvider router={router} />
        </TooltipProvider>
      </AdapterProvider>
    </IntlProvider>
  );
}
