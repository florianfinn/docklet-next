import { render } from "@testing-library/react";
import type { ReactNode } from "react";
import { createMemoryRouter, RouterProvider, type RouteObject } from "react-router";
import { IntlProvider } from "use-intl";

import { catalogs } from "@/app/i18n/catalog";
import { useAppLocale } from "@/app/i18n/locale-store";
import { AdapterProvider } from "@/data/AdapterProvider";
import type { DockletAdapter } from "@/data/adapter";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { TooltipProvider } from "@/shared/ui/tooltip";

/** Renders a memory router with the app providers. */
export function renderRoutes(routes: RouteObject[], initialEntries: string[] = ["/"], adapter: DockletAdapter = createDemoAdapter()) {
  const router = createMemoryRouter(routes, { initialEntries });
  const utils = render(
    <LocaleIntl>
      <AdapterProvider adapter={adapter}>
        <TooltipProvider>
          <RouterProvider router={router} />
        </TooltipProvider>
      </AdapterProvider>
    </LocaleIntl>,
  );
  return { ...utils, router };
}

/** Follows the locale store like App, so tests can switch the language live. */
function LocaleIntl({ children }: { children: ReactNode }) {
  const locale = useAppLocale();
  return (
    <IntlProvider locale={locale} messages={catalogs[locale]} timeZone="Europe/Berlin">
      {children}
    </IntlProvider>
  );
}

/** Renders a component at "/" with providers; other paths render their pathname. */
export function renderWithApp(ui: ReactNode, adapter: DockletAdapter = createDemoAdapter()) {
  return renderRoutes(
    [
      { path: "/", element: ui },
      { path: "*", element: <PathProbe /> },
    ],
    ["/"],
    adapter,
  );
}

function PathProbe() {
  return <p data-testid="path">{window.location.pathname}</p>;
}
