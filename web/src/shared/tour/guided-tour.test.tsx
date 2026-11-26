// Source: docklet-next/issues/640
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original guided tour coverage: offer, navigation pointer, page steps, fallback and placement.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AuthGate } from "@/app/AuthGate";
import { AppShell } from "@/app/shell/AppShell";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import { simulationStore } from "@/data/demo/simulation";
import { HelpSection } from "@/features/settings/sections/HelpSection";
import { helpStore } from "@/shared/preferences/help-store";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderRoutes } from "@/test/render";

import { placeCard } from "./place-card";
import { TOUR_STEPS } from "./tour-steps";
import { goToStep, offerTour, startTour, tourStore } from "./tour-store";

type HappyWindow = Window & { happyDOM: { setViewport: (v: { width: number; height: number }) => void } };
const setViewport = (width: number) => act(() => (window as unknown as HappyWindow).happyDOM.setViewport({ width, height: 800 }));

function Page({ name }: { name: string }) {
  return (
    <header>
      <h1>{name}</h1>
    </header>
  );
}

const pages = ["/dashboard", "/hosts", "/stacks", "/containers", "/security", "/backups", "/settings/notifications", "/settings/help"];
const routes = [{ element: <AppShell />, children: pages.map((path) => ({ path, element: <Page name={`Seite ${path}`} /> })) }];
const render = (path: string, adapter = createDemoAdapter()) => renderRoutes(routes, [path], adapter);
/** The real Explanations section with its unsaved-changes guard. */
const helpRoutes = [{ element: <AppShell />, children: [{ path: "/dashboard", element: <Page name="Seite /dashboard" /> }, { path: "/settings/help", element: <HelpSection /> }] }];

const card = () => screen.getByRole("dialog", { name: /./ });
const tourCard = () => document.querySelector<HTMLElement>('[data-slot="tour-card"]');
const overlay = () => document.querySelector<HTMLElement>('[data-slot="tour-overlay"]');
const spotlight = () => document.querySelector('[data-slot="tour-spotlight"]');
/** happy-dom has no layout; anchors count as visible only with a measured box. */
const measured = () =>
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({ x: 20, y: 20, top: 20, left: 20, right: 220, bottom: 60, width: 200, height: 40, toJSON: () => ({}) } as DOMRect);

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  vi.restoreAllMocks();
  tourStore.reset();
  setViewport(1280);
  helpStore.reset();
  preferencesStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

describe("guided tour", () => {
  it("offers the tour once and starts it, or closes the offer without starting", async () => {
    measured();
    render("/dashboard");
    act(() => offerTour());
    const offer = await screen.findByRole("dialog", { name: "Einrichtung abgeschlossen" });
    await userEvent.click(within(offer).getByRole("button", { name: "Nicht jetzt" }));
    expect(tourStore.get().status === "idle" && screen.queryByRole("dialog") === null).toBe(true);

    act(() => offerTour());
    await userEvent.click(await screen.findByRole("button", { name: "Tour starten" }));
    await waitFor(() => expect(overlay()?.dataset.phase).toBe("page"));
    expect(within(card()).getByRole("heading").textContent).toBe("Dashboard");
  });

  it("points at the navigation entry and continues on the page through Next, without navigating on its own", async () => {
    measured();
    const { router } = render("/settings/help");
    act(() => startTour());
    await waitFor(() => expect(overlay()?.dataset.phase).toBe("nav"));
    expect(router.state.location.pathname).toBe("/settings/help");
    expect(within(card()).getByText("Öffne „Dashboard“ in der Navigation links oder wähle Weiter.") !== null && spotlight() !== null).toBe(true);
    // The hole above the navigation entry stays clickable: four shields surround it, none covers it.
    expect(document.querySelectorAll('[data-slot="tour-shield"]').length).toBe(4);
    // The hole matches the entry exactly (top 20, left 20), without the visual padding that would expose neighbours.
    const [above, , beside] = document.querySelectorAll<HTMLElement>('[data-slot="tour-shield"]');
    expect([above?.style.height, beside?.style.width]).toEqual(["20px", "20px"]);

    await userEvent.click(within(card()).getByRole("button", { name: "Weiter" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"));
    await waitFor(() => expect(overlay()?.dataset.phase).toBe("page"));
    expect(within(card()).getByText(/^Die Lage aller Hosts auf einen Blick/) !== null).toBe(true);
    // On the page the highlight is closed as well.
    expect(document.querySelectorAll('[data-slot="tour-shield"]').length).toBe(5);
  });

  it("keeps the keyboard on the card and the highlighted entry while pointing", async () => {
    measured();
    const { router } = render("/settings/help");
    act(() => startTour());
    await waitFor(() => expect(overlay()?.dataset.phase).toBe("nav"));
    const inside = card();
    const link = document.querySelector<HTMLAnchorElement>('a[data-slot="sidebar-menu-button"][href="/dashboard"]');
    await waitFor(() => expect(document.activeElement === within(inside).getByRole("button", { name: "Weiter" })).toBe(true));
    // Shift+Tab cycles through End tour and the highlighted entry; nothing behind the shields is reached.
    const reached: boolean[] = [];
    for (let i = 0; i < 6; i++) {
      await userEvent.tab({ shift: true });
      reached.push(inside.contains(document.activeElement) || document.activeElement === link);
    }
    expect([reached.every(Boolean), router.state.location.pathname]).toEqual([true, "/settings/help"]);
    // From Next, Tab reaches the highlighted entry; the arrow key steps from there as from the card.
    await userEvent.tab();
    expect(document.activeElement === link).toBe(true);
    await userEvent.keyboard("{ArrowRight}");
    await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"));
  });

  it("leaves Escape to the search palette and resumes afterwards", async () => {
    measured();
    render("/dashboard");
    act(() => goToStep(TOUR_STEPS.length - 1));
    await waitFor(() => expect(within(card()).getByRole("heading").textContent).toBe("Suche und Aktivität"));
    await userEvent.keyboard("{Control>}k{/Control}");
    await waitFor(() => expect(overlay() === null && screen.queryByRole("dialog", { name: "Suchen" }) !== null).toBe(true));
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Suchen" })).toBe(null));
    expect(tourStore.get().status).toBe("running");
    await waitFor(() => expect(overlay() !== null).toBe(true));
  });

  it("continues when the user opens the page through the navigation link", async () => {
    measured();
    const { router } = render("/dashboard");
    act(() => goToStep(TOUR_STEPS.findIndex((s) => s.key === "hosts")));
    await waitFor(() => expect(overlay()?.dataset.phase).toBe("nav"));
    const link = document.querySelector<HTMLAnchorElement>('a[data-slot="sidebar-menu-button"][href="/hosts"]');
    expect(link !== null).toBe(true);
    await userEvent.click(link as HTMLAnchorElement);
    await waitFor(() => expect(router.state.location.pathname).toBe("/hosts"));
    await waitFor(() => expect(overlay()?.dataset.phase).toBe("page"));
    expect(within(card()).getByText(/^Hier verbindest du weitere Docker-Hosts/) !== null).toBe(true);
  });

  it("advances on the same page, goes back with Back and ends with Escape", async () => {
    measured();
    render("/dashboard");
    act(() => startTour());
    await waitFor(() => expect(overlay()?.dataset.phase).toBe("page"));
    await userEvent.click(within(card()).getByRole("button", { name: "Weiter" }));
    await waitFor(() => expect(within(card()).getByRole("heading").textContent).toBe("Hostfilter"));
    expect(within(card()).getByText("Schritt 2 von 9") !== null).toBe(true);
    await userEvent.click(within(card()).getByRole("button", { name: "Zurück" }));
    await waitFor(() => expect(within(card()).getByRole("heading").textContent).toBe("Dashboard"));
    // The page step is modal: Tab moves from Next, the last button, back to the first one.
    await waitFor(() => expect(document.activeElement === within(card()).getByRole("button", { name: "Weiter" })).toBe(true));
    await userEvent.tab();
    expect(document.activeElement === within(card()).getByRole("button", { name: "Tour beenden" })).toBe(true);
    await userEvent.keyboard("{Escape}");
    expect(tourStore.get().status === "idle" && overlay() === null).toBe(true);
    // Without a remaining opener, focus returns to the page.
    await waitFor(() => expect(document.activeElement?.id).toBe("main-content"));
  });

  it("finishes on the last step", async () => {
    measured();
    render("/settings/notifications");
    act(() => goToStep(TOUR_STEPS.length - 1));
    await waitFor(() => expect(within(card()).getByRole("heading").textContent).toBe("Suche und Aktivität"));
    await userEvent.click(within(card()).getByRole("button", { name: "Fertig" }));
    expect(tourStore.get().status === "idle" && overlay() === null).toBe(true);
  });

  it("points at the menu button on phones and keeps the tour usable inside the opened menu", async () => {
    measured();
    setViewport(390);
    const { router } = render("/settings/help");
    act(() => startTour());
    await waitFor(() => expect(tourCard()?.textContent?.includes("Öffne oben links das Menü und dann „Dashboard“ oder wähle Weiter.")).toBe(true));
    await userEvent.click(document.querySelector<HTMLElement>('[data-slot="sidebar-trigger"]') as HTMLElement);
    // The open menu is a modal sheet; the card lives inside it, so it stays reachable and focused.
    await waitFor(() => expect(tourCard()?.closest('[data-mobile="true"]') != null).toBe(true));
    const inside = tourCard() as HTMLElement;
    await waitFor(() => expect(document.activeElement === within(inside).getByRole("button", { name: "Weiter" })).toBe(true));
    expect([inside.closest('[aria-hidden="true"]') === null, inside.textContent?.includes("Öffne „Dashboard“ in der Navigation links oder wähle Weiter.")]).toEqual([true, true]);
    await userEvent.keyboard("{ArrowRight}");
    await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"));
    await waitFor(() => expect(overlay()?.dataset.phase).toBe("page"));
  });

  it("keeps focus inside the open phone menu when the tour ends there", async () => {
    measured();
    setViewport(390);
    renderRoutes(helpRoutes, ["/settings/help"]);
    await userEvent.click(await screen.findByRole("button", { name: "Tour starten" }));
    await waitFor(() => expect(tourCard() !== null).toBe(true));
    await userEvent.click(document.querySelector<HTMLElement>('[data-slot="sidebar-trigger"]') as HTMLElement);
    await waitFor(() => expect(tourCard()?.closest('[data-mobile="true"]') != null).toBe(true));
    await userEvent.click(within(tourCard() as HTMLElement).getByRole("button", { name: "Tour beenden" }));
    // The opener lies behind the modal menu; focus stays in the menu instead.
    expect(tourStore.get().status).toBe("idle");
    await waitFor(() => expect(document.querySelector('[data-mobile="true"][data-state="open"]')?.contains(document.activeElement)).toBe(true));
  });

  it("returns focus to the opener when Escape closes the tour and the phone menu together", async () => {
    measured();
    setViewport(390);
    renderRoutes(helpRoutes, ["/settings/help"]);
    const opener = await screen.findByRole("button", { name: "Tour starten" });
    await userEvent.click(opener);
    await waitFor(() => expect(tourCard() !== null).toBe(true));
    await userEvent.click(document.querySelector<HTMLElement>('[data-slot="sidebar-trigger"]') as HTMLElement);
    await waitFor(() => expect(tourCard()?.closest('[data-mobile="true"]') != null).toBe(true));
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect([tourStore.get().status, document.querySelector('[data-mobile="true"]') === null]).toEqual(["idle", true]));
    await waitFor(() => expect(document.activeElement === opener).toBe(true));
  });

  it("recovers focus when the focused entry disappears with a viewport change", async () => {
    measured();
    render("/settings/help");
    act(() => startTour());
    await waitFor(() => expect(overlay()?.dataset.phase).toBe("nav"));
    await waitFor(() => expect(document.activeElement === within(card()).getByRole("button", { name: "Weiter" })).toBe(true));
    await userEvent.tab();
    expect(document.activeElement?.getAttribute("href")).toBe("/dashboard");
    setViewport(390);
    await waitFor(() => expect(document.activeElement === within(tourCard() as HTMLElement).getByRole("button", { name: "Weiter" })).toBe(true));
  });

  it("waits behind the discard confirmation of an unsaved settings draft", async () => {
    measured();
    const { router } = renderRoutes(helpRoutes, ["/settings/help"]);
    await userEvent.click(within(await screen.findByRole("radiogroup", { name: "Erklärungen" })).getByRole("radio", { name: "Kurz" }));
    await userEvent.click(screen.getByRole("button", { name: "Tour starten" }));
    await waitFor(() => expect(overlay()?.dataset.phase).toBe("nav"));
    await userEvent.click(within(card()).getByRole("button", { name: "Weiter" }));
    // The tour steps aside while the confirmation is open, then continues on the opened page.
    const confirm = await screen.findByRole("alertdialog");
    expect(overlay() === null && router.state.location.pathname === "/settings/help").toBe(true);
    await userEvent.click(within(confirm).getByRole("button", { name: "Verwerfen und wechseln" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"));
    await waitFor(() => expect(overlay()?.dataset.phase).toBe("page"));
  });

  it("clears the highlight when the anchor disappears", async () => {
    measured();
    function Vanishing() {
      const [shown, setShown] = useState(true);
      return shown ? <><Page name="Seite /hosts" /><button type="button" onClick={() => setShown(false)}>Ausblenden</button></> : <p>leer</p>;
    }
    renderRoutes([{ element: <AppShell />, children: [{ path: "/hosts", element: <Vanishing /> }] }], ["/hosts"]);
    act(() => goToStep(TOUR_STEPS.findIndex((s) => s.key === "hosts")));
    await waitFor(() => expect(spotlight() !== null).toBe(true));
    // happy-dom has no hit testing; the click stands in for the page replacing its content.
    await userEvent.click(screen.getByRole("button", { name: "Ausblenden" }));
    await waitFor(() => expect(spotlight() === null && document.querySelectorAll('[data-slot="tour-shield"]').length === 0).toBe(true));
    expect(tourCard()?.classList.contains("opacity-0")).toBe(false);
  });

  it("ends when the signed-in identity changes", async () => {
    measured();
    const demo = createDemoAdapter();
    const adapter = { ...demo, subscribeChanges: demo.subscribeChanges.bind(demo) };
    let notify = () => {};
    registerIdentityLifetime(adapter.subscribeChanges, { version: () => 0, active: () => true, listen: (n) => { notify = n; return () => {}; } });
    render("/dashboard", adapter);
    act(() => startTour());
    await waitFor(() => expect(overlay() !== null).toBe(true));
    act(() => notify());
    expect(tourStore.get().status === "idle" && overlay() === null).toBe(true);
  });

  it("shows the card centred without a highlight when no anchor is visible", async () => {
    render("/dashboard");
    act(() => goToStep(TOUR_STEPS.findIndex((s) => s.key === "hostFilter")));
    await waitFor(() => expect(card().classList.contains("opacity-0")).toBe(false), { timeout: 3000 });
    expect(spotlight() === null && document.querySelectorAll('[data-slot="tour-shield"]').length === 0).toBe(true);
  });

  it("is not offered to simple users", async () => {
    const demo = createDemoAdapter();
    const adapter = { ...demo, kind: "http" as const, getCurrentUser: vi.fn(async () => ({ id: "member", username: "member", isAdmin: false, assignments: [], requiresPasswordChange: false })) };
    renderRoutes([{ element: <AuthGate />, children: routes }], ["/dashboard"], adapter);
    expect(await screen.findByRole("heading", { name: "Seite /dashboard" }) !== null).toBe(true);
    act(() => offerTour());
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByRole("dialog", { name: "Einrichtung abgeschlossen" })).toBe(null);
  });
});

describe("card placement", () => {
  const rect = (left: number, top: number, width: number, height: number) => ({ left, top, width, height, right: left + width, bottom: top + height }) as DOMRect;
  const w = 300, h = 150, vw = 1200, vh = 800;

  it("prefers below, then right, above and left of the highlight", () => {
    expect(placeCard(rect(100, 100, 200, 40), w, h, vw, vh)).toEqual({ left: 92, top: 160 });
    expect(placeCard(rect(100, 500, 200, 280), w, h, vw, vh)).toEqual({ left: 320, top: 492 });
    expect(placeCard(rect(800, 600, 380, 180), w, h, vw, vh)).toEqual({ left: 792, top: 430 });
    expect(placeCard(rect(700, 20, 480, 760), w, h, vw, vh)).toEqual({ left: 380, top: 16 });
    // A highlight scrolled above the viewport keeps the card on screen.
    expect(placeCard(rect(100, -300, 200, 40), w, h, vw, vh)).toEqual({ left: 92, top: 16 });
  });

  it("centres without a highlight and overlaps at the bottom right when nothing fits", () => {
    expect(placeCard(null, w, h, vw, vh)).toEqual({ left: 450, top: 325 });
    expect(placeCard(rect(0, 0, 1200, 800), w, h, vw, vh)).toEqual({ left: 884, top: 634 });
  });
});
