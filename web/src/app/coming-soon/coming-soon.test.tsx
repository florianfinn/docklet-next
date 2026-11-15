// Source: docklet-next/issues/359
// Revision: 4e71e45a2297a4f2719188cdf15c3e4d586b5357
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original production route proof without an adapter context or network reads; optional-module foundation (#472).

import { cleanup, render, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider, type RouteObject } from "react-router";
import { Suspense } from "react";
import { IntlProvider } from "use-intl";
import { afterAll, afterEach, beforeAll, expect, it, vi } from "vitest";
vi.mock("@/runtime", () => vi.importActual("../../runtime"));
let routes: RouteObject[];
let catalogs: typeof import("@/app/i18n/catalog").catalogs;
beforeAll(async () => {
  vi.stubEnv("DEV", false); vi.resetModules();
  ({ routes } = await import("@/app/router"));
  ({ catalogs } = await import("@/app/i18n/catalog"));
}, 60_000);
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
afterAll(() => vi.unstubAllEnvs());
function find(path: string, items = routes): RouteObject | undefined {
  for (const route of items) { if (route.path === path) return route; const nested = find(path, route.children ?? []); if (nested) return nested; }
}
it.each(["proxy", "games"])("production /%s renders with zero adapter requests", async (area) => {
  const fetch = vi.fn(); vi.stubGlobal("fetch", fetch);
  for (const locale of ["en-US", "de"] as const) {
    const declared = find(area); expect(declared !== undefined).toBe(area === "games");
    const route=declared ?? find("*")!;
    const router = createMemoryRouter([route], { initialEntries: [`/${area}`] });
    const view = render(<IntlProvider locale={locale} messages={catalogs[locale]} timeZone="UTC"><Suspense fallback={null}><RouterProvider router={router} /></Suspense></IntlProvider>);
    if (area === "proxy") await screen.findByText(catalogs[locale].shell.notFound.title);
    else {
      await screen.findByRole("heading", { name: "Games" });
      expect(screen.getByText(locale === "de" ? "Bald verfügbar" : "Coming soon") !== null).toBe(true);
    }
    const back = area === "proxy" ? catalogs[locale].shell.notFound.back : locale === "de" ? "Zurück zum Dashboard" : "Back to dashboard";
    expect(screen.getByRole("link", { name: back }).getAttribute("href")).toBe("/dashboard");
    expect(screen.queryByRole("button") === null).toBe(true);
    expect(fetch).not.toHaveBeenCalled();
    view.unmount();
  }
});
