// Source: docklet-next/issues/462
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original shell request, value, partial-host, lifecycle and immutable-cache regressions.

import { act, screen, waitFor, fireEvent } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { renderWithApp, renderRoutes } from "@/test/render";
import { setAppLocale } from "@/app/i18n/locale-store";
import { setPreference, preferencesStore } from "@/shared/preferences/preferences-store";
import { SidebarProvider } from "@/shared/ui/sidebar";
import { FreeSidebar } from "./free/FreeSidebar";
import { NAV } from "./nav-items";
import DashboardPage from "@/features/dashboard/DashboardPage";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/data/query-keys";
import { navSecurityCount } from "./nav-count-values";
import { useNavCounts } from "./use-nav-counts";
import { useNavInventory } from "./use-nav-inventory";
import { useActiveSecurity } from "@/data/queries";
import { navFixture } from "./nav-counts.test-support";

function NavProbe() {
  const counts = useNavCounts(), inventory = useNavInventory(), security = useActiveSecurity();
  return <><output data-testid="counts">{JSON.stringify(counts)}</output><output data-testid="ready">{String(inventory.isSuccess && security.isSuccess && !inventory.isFetching && !security.isFetching)}</output></>;
}

function RecoverInventory() {
  const client = useQueryClient();
  return <button onClick={() => void client.invalidateQueries({ queryKey: queryKeys.navInventory })}>Recover inventory</button>;
}

const counts = () => JSON.parse(screen.getByTestId("counts").textContent!);
const ready = () => waitFor(() => expect(screen.getByTestId("ready").textContent).toBe("true"));
afterEach(() => { preferencesStore.reset(); setAppLocale("de"); });

it("row 7: reads bounded shell sources on a non-security route for two hosts", async () => {
  const f = navFixture();
  renderRoutes([{ path: "/containers", element: <SidebarProvider><FreeSidebar navigation={NAV} /><NavProbe /></SidebarProvider> }], ["/containers"], f.adapter);
  await ready();
  expect(counts().security.value).toBe(4);
  const paths = f.fetcher.mock.calls.map(([url]) => new URL(String(url), "https://hub.example.com").pathname);
  for (const host of ["first", "second"]) {
    expect(paths.filter((path) => path === `/api/environments/${host}/images/vulnerabilities/active-summaries`)).toHaveLength(1);
    expect(paths.filter((path) => path === `/api/environments/${host}/volumes/_collection/counts`)).toHaveLength(1);
  }
  expect(paths.filter((path) => /\/(vulnerabilities\/all|summaries|volumes|volumes\/_collection\/binds|overview)$/.test(path))).toEqual([]);
});

it("row 8: keeps running-only security including system consumers and distinct colon-containing binds with host filtering", async () => {
  const f = navFixture(); f.state.extras = true;
  renderWithApp(<NavProbe />, f.adapter); await ready();
  expect(counts().security).toEqual({ value: 8, partial: false, tone: "err" });
  expect(counts().volumes.value).toBe(7);
  act(() => setPreference("hostSelection", ["second"]));
  expect(counts().security.value).toBe(4);
  expect(counts().volumes.value).toBe(4);
  const inventory = await f.adapter.listNavInventory();
  expect(inventory[0]?.containers[0]?.bindSources).toEqual(["/srv/example:data", "/srv/example:data"]);
});

it.each(["en-US", "de"] as const)("row 10: inventory failure with successful summaries is partial and recovers (%s)", async (locale) => {
  setAppLocale(locale); const f = navFixture(); f.state.inventoryFailures.add("second");
  renderWithApp(<SidebarProvider><FreeSidebar navigation={NAV} /><NavProbe /><RecoverInventory /></SidebarProvider>, f.adapter); await ready();
  expect(counts().security).toEqual({ value: 2, partial: true, tone: "err" });
  const title = locale === "de" ? "Unvollständige Sicherheitssumme: Scan-Daten für mindestens einen Host sind nicht verfügbar." : "Partial security total: scan data is unavailable for one or more hosts.";
  expect(screen.getByLabelText(title).textContent).toBe("2+");
  act(() => setPreference("hostSelection", ["second"]));
  expect(counts().security).toEqual({ value: "?", partial: true });
  f.state.inventoryFailures.clear();
  fireEvent.click(screen.getByRole("button", { name: "Recover inventory" }));
  await waitFor(() => expect(counts().security).toEqual({ value: 2, partial: false, tone: "err" }));
});

it.each(["count", "inventory"])("row 11: retains the last combined total when the %s leg fails, then recovers", async (leg) => {
  const f = navFixture();
  const initial = await f.adapter.listNavInventory();
  expect(initial.find((h) => h.hostId === "second")?.volumeTotal).toBe(4);
  f.state.count.second = 20; f.state.bindSources.push("/srv/another");
  (leg === "count" ? f.state.countFailures : f.state.inventoryFailures).add("second");
  const partial = await f.adapter.listNavInventory();
  expect(partial.find((h) => h.hostId === "second")?.volumeTotal).toBe(4);
  expect(partial.find((h) => h.hostId === "first")?.volumeTotal).toBe(4);
  f.state.countFailures.clear(); f.state.inventoryFailures.clear();
  const recovered = await f.adapter.listNavInventory();
  expect(recovered.find((h) => h.hostId === "second")?.volumeTotal).toBe(22);
});

it("shares the Dashboard active summaries query with the mounted shell", async () => {
  const f = navFixture();
  renderWithApp(<><NavProbe /><DashboardPage /></>, f.adapter); await ready();
  expect(f.fetcher.mock.calls.filter(([url]) => String(url).endsWith("/active-summaries"))).toHaveLength(2);
});

it("row 12: consumes frozen shared active summaries and raw containers without mutation", async () => {
  const f = navFixture(), security = await f.adapter.getActiveSecurity(), inventory = await f.adapter.listNavInventory();
  const freeze = <T,>(value: T): T => { if (value && typeof value === "object") { Object.values(value).forEach(freeze); Object.freeze(value); } return value; };
  const before = JSON.stringify({ security, inventory }); freeze(security); freeze(inventory);
  expect(() => navSecurityCount(security, inventory, [])).not.toThrow();
  expect(navSecurityCount(security, inventory, [])).toEqual({ critical: 4, partial: false });
  function Cached() {
    const client = useQueryClient();
    client.setQueryDefaults(queryKeys.activeSecurity, { staleTime: Infinity });
    client.setQueryDefaults(queryKeys.navInventory, { staleTime: Infinity });
    client.setQueryData(queryKeys.activeSecurity, security); client.setQueryData(queryKeys.navInventory, inventory);
    return <NavProbe />;
  }
  renderWithApp(<Cached />, f.adapter); await ready();
  expect(counts().security.value).toBe(4); expect(counts().volumes.value).toBe(7);
  expect(JSON.stringify({ security, inventory })).toBe(before);
});
