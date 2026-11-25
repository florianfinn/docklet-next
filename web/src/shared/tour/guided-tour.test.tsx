// Source: docklet-next/issues/640
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original guided tour coverage: offer, navigation pointer, page steps, fallback and placement.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AuthGate } from "@/app/AuthGate";
import { AppShell } from "@/app/shell/AppShell";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
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

const card = () => screen.getByRole("dialog", { name: /./ });
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

    await userEvent.click(within(card()).getByRole("button", { name: "Weiter" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"));
    await waitFor(() => expect(overlay()?.dataset.phase).toBe("page"));
    expect(within(card()).getByText(/^Die Lage aller Hosts auf einen Blick/) !== null).toBe(true);
    // On the page the highlight is closed as well.
    expect(document.querySelectorAll('[data-slot="tour-shield"]').length).toBe(5);
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
    await userEvent.keyboard("{Escape}");
    expect(tourStore.get().status === "idle" && overlay() === null).toBe(true);
  });

  it("finishes on the last step", async () => {
    measured();
    render("/settings/notifications");
    act(() => goToStep(TOUR_STEPS.length - 1));
    await waitFor(() => expect(within(card()).getByRole("heading").textContent).toBe("Suche und Aktivität"));
    await userEvent.click(within(card()).getByRole("button", { name: "Fertig" }));
    expect(tourStore.get().status === "idle" && overlay() === null).toBe(true);
  });

  it("points at the menu button while the mobile navigation is closed", async () => {
    measured();
    setViewport(390);
    render("/settings/help");
    act(() => startTour());
    await waitFor(() => expect(within(card()).getByText("Öffne oben links das Menü und dann „Dashboard“ oder wähle Weiter.") !== null).toBe(true));
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
  });

  it("centres without a highlight and overlaps at the bottom right when nothing fits", () => {
    expect(placeCard(null, w, h, vw, vh)).toEqual({ left: 450, top: 325 });
    expect(placeCard(rect(0, 0, 1200, 800), w, h, vw, vh)).toEqual({ left: 884, top: 634 });
  });
});
