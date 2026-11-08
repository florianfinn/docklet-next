// Source: docklet-next/issues/359
// Revision: 4e71e45a2297a4f2719188cdf15c3e4d586b5357
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original production route proof without an adapter context or network reads.

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
    const route = find(area)!; expect(route !== undefined).toBe(true);
    const router = createMemoryRouter([route], { initialEntries: [`/${area}`] });
    const view = render(<IntlProvider locale={locale} messages={catalogs[locale]} timeZone="UTC"><Suspense fallback={null}><RouterProvider router={router} /></Suspense></IntlProvider>);
    await screen.findByRole("heading", { name: area === "proxy" ? "Proxy" : "Games" });
    expect(screen.getByText(locale === "de" ? "Bald verfügbar" : "Coming soon") !== null).toBe(true);
    expect(screen.getByRole("link", { name: locale === "de" ? "Zurück zum Dashboard" : "Back to dashboard" }).getAttribute("href")).toBe("/dashboard");
    expect(screen.queryByRole("button") === null).toBe(true);
    expect(fetch).not.toHaveBeenCalled();
    view.unmount();
  }
});
