// Source: docklet-next/issues/528
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 coverage of the modules navigation group (U1, break-map row U-1).

import { screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";

import { AppShell } from "./AppShell";
import { effectiveNavigation, NAV } from "./nav-items";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => {
  setAppLocale("de");
  simulationStore.reset();
  localStorage.clear();
});

const routes = [{ element: <AppShell />, children: [{ path: "/dashboard", element: <h1>Dashboard</h1> }] }];
const proxyLink = () => document.querySelector('a[href="/proxy"]');

it("U-1 projects an enabled module into the modules group and drops it when disabled", () => {
  const on = effectiveNavigation(NAV, true, { proxy: true });
  expect(on.find((g) => g.key === "modules")?.items.map((i) => [i.key, i.comingSoon === true])).toEqual([["proxy", false]]);
  expect(on.map((g) => g.key).indexOf("modules")).toBe(on.map((g) => g.key).indexOf("system") + 1);
  expect(on.find((g) => g.key === "later")?.items.map((i) => i.key)).toEqual(["games"]);
  for (const modules of [{ proxy: false }, undefined]) {
    const off = effectiveNavigation(NAV, true, modules);
    expect(off.some((g) => g.key === "modules")).toBe(false);
    expect(off.flatMap((g) => g.items).some((i) => i.key === "proxy")).toBe(false);
  }
});

it.each([
  ["en-US", "Modules"],
  ["de", "Module"],
] as const)("U-1 sidebar shows the enabled proxy module under %s without a coming-soon marker", async (locale, label) => {
  setAppLocale(locale);
  const adapter = createDemoAdapter();
  await adapter.setModuleEnabled!("proxy", true);
  renderRoutes(routes, ["/dashboard"], adapter);
  await waitFor(() => expect(proxyLink() !== null).toBe(true));
  expect(proxyLink()?.hasAttribute("data-coming-soon")).toBe(false);
  expect(screen.getByText(label) !== null).toBe(true);
});

it("U-1 sidebar hides a disabled module and its empty group", async () => {
  setAppLocale("en-US");
  const adapter = createDemoAdapter();
  await adapter.setModuleEnabled!("proxy", false);
  renderRoutes(routes, ["/dashboard"], adapter);
  await screen.findByText("System");
  await waitFor(() => expect(document.querySelector('a[href="/games"]') !== null).toBe(true));
  expect(proxyLink() === null).toBe(true);
  expect(screen.queryByText("Modules") === null).toBe(true);
});
