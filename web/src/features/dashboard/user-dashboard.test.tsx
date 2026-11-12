// Source: docklet-next/docs/design/service-widgets.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original reduced user dashboard test for issue #332 (decision K6, SW9 real user session).

import { screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { routes } from "@/app/router";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";

beforeEach(() => { setAppLocale("en-US"); simulationStore.set((s) => ({ ...s, latency: "instant", live: false })); });
afterEach(() => { setAppLocale("de"); simulationStore.reset(); });

it("K6 lands a simple user on a reduced dashboard with enabled service widgets and apps, without infrastructure reads or actions", async () => {
  const demo = createDemoAdapter();
  const [stack] = await demo.listStacks();
  await demo.saveServiceWidgetConfig("plex", { enabled: false, address: "http://192.0.2.21:32400", openUrl: "", credentialAction: "keep" });
  await demo.saveServiceWidgetConfig("minecraft", { enabled: true, address: "", openUrl: "", credentialAction: "keep" });
  const adapter = {
    ...demo, kind: "http" as const,
    getCurrentUser: vi.fn(async () => ({ id: "member", username: "member", isAdmin: false, assignments: [], requiresPasswordChange: false })),
    listStacks: vi.fn(async () => [{ ...stack!, url: "https://app.example.com" }]),
    listActions: async () => [], subscribeActions: () => () => {},
  };
  const infrastructure = (["listContainers", "getSecurity", "listSchedule", "getCapacity", "listEvents", "getConnectedSettings", "getServiceWidgetConfig"] as const).map((name) => vi.spyOn(adapter, name));
  renderRoutes(routes, ["/"], adapter);
  await screen.findByRole("heading", { name: "Dashboard" });
  const nav = [...document.querySelectorAll("[data-sidebar=menu] a")].map((a) => a.getAttribute("href"));
  expect(nav).toEqual(["/dashboard", "/stacks"]);
  await waitFor(() => expect(document.querySelector("[data-user-widget=overseerr]") !== null).toBe(true));
  // Turned off (plex) and not set up (minecraft) widgets stay hidden from users.
  expect([...document.querySelectorAll("[data-user-widget]")].map((el) => el.getAttribute("data-user-widget"))).toEqual(["overseerr", "apps"]);
  const overseerr = document.querySelector<HTMLElement>("[data-user-widget=overseerr]")!;
  await waitFor(() => expect(within(overseerr).queryAllByRole("listitem").length).toBeGreaterThan(0));
  expect(within(overseerr).queryAllByRole("button", { name: /^(Approve|Decline) / })).toEqual([]);
  const apps = document.querySelector<HTMLElement>("[data-user-widget=apps]")!;
  expect((await within(apps).findByRole("link", { name: new RegExp(stack!.name) })).getAttribute("href")).toBe("https://app.example.com");
  expect(screen.queryByRole("link", { name: "Set up" })).toBe(null);
  for (const spy of infrastructure) expect(spy).not.toHaveBeenCalled();
});
