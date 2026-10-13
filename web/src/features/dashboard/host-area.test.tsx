import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { applySnapshot, captureSnapshot, decodeDesignCode, encodeDesignCode } from "@/features/design-lab/favorite-code";
import { renderWithApp } from "@/test/render";

import DashboardPage from "./DashboardPage";
import { DEFAULT_LAYOUT, layoutStore, mergeLayout, visibleWidgets, widgetSize } from "./dashboard-layout";
import { fleetViewStore, setFleetView } from "./fleet-view-store";
import { hostAreaMinHeightRem, snapHostArea } from "./use-host-area-resize";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  vi.restoreAllMocks();
  layoutStore.reset();
  fleetViewStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

const area = () => document.querySelector<HTMLElement>('[data-slot="host-cards"]');
const storedFleet = () => JSON.parse(localStorage.getItem("docklet-react-opus.fleet-view") ?? "{}").data;

async function editDashboard() {
  renderWithApp(<DashboardPage />);
  await waitFor(() => expect(document.querySelector("[data-host-card='atlas']") !== null).toBe(true));
  await userEvent.click(screen.getByRole("button", { name: "Anpassen" }));
}

describe("factory dashboard layout", () => {
  it("starts with attention, a wide security band with apps below and tall service columns", () => {
    expect(visibleWidgets(DEFAULT_LAYOUT)).toEqual(["attention", "security", "events", "overseerr", "plex", "schedule", "apps"]);
    expect(["attention", "security", "apps", "events"].map((id) => widgetSize(DEFAULT_LAYOUT, id as "attention"))).toEqual([
      { w: 1, h: 6 },
      { w: 2, h: 3 },
      { w: 2, h: 3 },
      { w: 1, h: 6 },
    ]);
    // No service is connected by default; demo content needs an explicit connect.
    expect(Object.values(DEFAULT_LAYOUT.services).map((s) => s.status)).toEqual(["unconfigured", "unconfigured", "unconfigured"]);
  });

  it("never replaces a stored custom layout with the new factory sizes or order", () => {
    const custom = { order: ["load", "attention", "timeline"], hidden: ["security"], sizes: { load: { w: 2, h: 5 } }, services: {} };
    const merged = mergeLayout(custom, DEFAULT_LAYOUT);
    expect([merged.order.slice(0, 3), merged.hidden.includes("security"), merged.sizes]).toEqual([["load", "attention", "timeline"], true, { load: { w: 2, h: 5 } }]);
    expect(widgetSize(merged, "attention")).toEqual({ w: 1, h: 4 });
  });

  it("shows the unconfigured services honestly on a fresh dashboard", async () => {
    renderWithApp(<DashboardPage />);
    const plex = await waitFor(() => document.querySelector<HTMLElement>('[data-widget="plex"]') ?? Promise.reject(new Error("plex missing")));
    expect(within(plex).getByText("Konfiguration fehlt") !== null).toBe(true);
    expect(within(plex).queryByRole("list", { name: "Aktive Streams" })).toBe(null);
    expect(document.querySelector('[data-widget-slot="security"]')?.getAttribute("data-size")).toBe("2x3");
  });
});

describe("host area size", () => {
  it("anchors the host area at the bottom beside the visual and lets header and tiles take the edge light", async () => {
    act(() => setFleetView({ placement: "right" }));
    renderWithApp(<DashboardPage />);
    await waitFor(() => expect(document.querySelector("[data-host-card='atlas']") !== null).toBe(true));
    const column = document.querySelector('[data-slot="host-cards-column"]');
    const summary = document.querySelector('[data-slot="ops-summary"]');
    const tile = document.querySelector("[data-host-card='atlas'] article");
    // Bottom alignment comes from the grid (self-end), never from a pixel offset.
    expect([column?.className.includes("self-end"), column?.getAttribute("style"), summary?.hasAttribute("data-glow"), tile?.hasAttribute("data-glow")]).toEqual([true, null, true, true]);
    act(() => setFleetView({ placement: "below" }));
    expect(document.querySelector('[data-slot="host-cards-column"]')?.className.includes("self-end")).toBe(false);
  });

  it("snaps pointer deltas to twelfths and dashboard rows within its limits", () => {
    const cell = { twelfth: 100, row: 96 };
    // Handle on the left edge: dragging left widens the area beside the visual.
    expect(snapHostArea({ w: 7, h: 2 }, { x: -210, y: 190 }, cell, true)).toEqual({ w: 9, h: 4 });
    // One row is the flat view; nothing goes below it.
    expect(snapHostArea({ w: 7, h: 2 }, { x: 900, y: -900 }, cell, true)).toEqual({ w: 4, h: 1 });
    expect(snapHostArea({ w: 7, h: 2 }, { x: 5000, y: 5000 }, cell, false)).toEqual({ w: 9, h: 8 });
    expect([hostAreaMinHeightRem(1), hostAreaMinHeightRem(2), hostAreaMinHeightRem(4)]).toEqual([5, 11, 23]);
  });

  it("resizes by Alt + Shift + arrows on a card: only the height below the visual, width beside it", async () => {
    act(() => setFleetView({ placement: "below" }));
    await editDashboard();
    const card = document.querySelector<HTMLElement>("[data-host-card='atlas'] [tabindex='0']") as HTMLElement;
    card.focus();
    fireEvent.keyDown(card, { key: "ArrowDown", altKey: true, shiftKey: true });
    fireEvent.keyDown(card, { key: "ArrowRight", altKey: true, shiftKey: true });
    expect(storedFleet().hostArea).toEqual({ w: 5, h: 3 });
    expect([area()?.dataset.area, area()?.style.minHeight]).toEqual(["5x3", "17rem"]);
    expect(screen.getByText("Hostbereich: 5 von 12 Spalten × 3 Reihen") !== null).toBe(true);
    // Moving cards still works with Alt alone; the order is not touched by resizing.
    expect(fleetViewStore.get().hostOrder).toEqual([]);

    act(() => setFleetView({ placement: "right" }));
    fireEvent.keyDown(card, { key: "ArrowRight", altKey: true, shiftKey: true });
    fireEvent.keyDown(card, { key: "ArrowUp", altKey: true, shiftKey: true });
    expect(fleetViewStore.get().hostArea).toEqual({ w: 6, h: 2 });
    expect((document.querySelector('[data-slot="ops-layout"]') as HTMLElement).style.getPropertyValue("--host-share")).toBe("6fr");
  });

  it("resizes with the size popup and the keyboard-operable corner handle; Escape cancels a drag", async () => {
    act(() => setFleetView({ placement: "right" }));
    await editDashboard();
    await userEvent.click(screen.getByRole("button", { name: "Größe von Hostbereich: 5 × 2" }));
    const popup = await screen.findByRole("dialog", { name: "Größe von Hostbereich" });
    await userEvent.click(within(popup).getByRole("button", { name: "Breiter" }));
    await userEvent.click(within(popup).getByRole("button", { name: "Höher" }));
    expect(fleetViewStore.get().hostArea).toEqual({ w: 6, h: 3 });
    await userEvent.click(within(popup).getByRole("button", { name: "Standardgröße" }));
    expect(fleetViewStore.get().hostArea).toEqual({ w: 5, h: 2 });
    await userEvent.keyboard("{Escape}");

    const handle = screen.getByRole("button", { name: "Größe von Hostbereich ändern (ziehen oder Pfeiltasten)" });
    fireEvent.keyDown(handle, { key: "ArrowDown" });
    expect(fleetViewStore.get().hostArea).toEqual({ w: 5, h: 3 });

    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(function (this: HTMLElement) {
      return this.dataset.slot === "ops-summary" ? 1200 : 0;
    });
    fireEvent.pointerDown(handle, { button: 0, pointerId: 3, clientX: 500, clientY: 300, pointerType: "mouse" });
    act(() => {
      window.dispatchEvent(new PointerEvent("pointermove", { pointerId: 3, clientX: 300, clientY: 400 }));
    });
    // Live preview of two twelfths wider and one row higher, not yet saved.
    expect([area()?.dataset.area, fleetViewStore.get().hostArea]).toEqual(["7x4", { w: 5, h: 3 }]);
    act(() => {
      window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    });
    expect([area()?.dataset.area, fleetViewStore.get().hostArea]).toEqual(["5x3", { w: 5, h: 3 }]);

    fireEvent.pointerDown(handle, { button: 0, pointerId: 4, clientX: 500, clientY: 300, pointerType: "mouse" });
    act(() => {
      window.dispatchEvent(new PointerEvent("pointermove", { pointerId: 4, clientX: 400, clientY: 300 }));
    });
    act(() => {
      window.dispatchEvent(new PointerEvent("pointerup", { pointerId: 4, clientX: 400, clientY: 300 }));
    });
    expect(storedFleet().hostArea).toEqual({ w: 6, h: 3 });
    // The card order did not change by resizing.
    expect(fleetViewStore.get().hostOrder).toEqual([]);
  });

  it("goes flat to one row and changes width and height independently with the edge handles", async () => {
    await editDashboard();
    const heightHandle = screen.getByRole("button", { name: "Höhe von Hostbereich ändern (ziehen oder Pfeil hoch/runter)" });
    const widthHandle = screen.getByRole("button", { name: "Breite von Hostbereich ändern (ziehen oder Pfeil links/rechts)" });
    fireEvent.keyDown(heightHandle, { key: "ArrowUp" });
    // Arrows of the other axis do nothing on an edge handle.
    fireEvent.keyDown(heightHandle, { key: "ArrowRight" });
    fireEvent.keyDown(widthHandle, { key: "ArrowDown" });
    expect([fleetViewStore.get().hostArea, area()?.dataset.area, area()?.style.minHeight]).toEqual([{ w: 5, h: 1 }, "5x1", "5rem"]);
    fireEvent.keyDown(widthHandle, { key: "ArrowRight" });
    expect(storedFleet().hostArea).toEqual({ w: 6, h: 1 });

    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(function (this: HTMLElement) {
      return this.dataset.slot === "ops-summary" ? 1200 : 0;
    });
    // A diagonal drag on the width handle changes only the width.
    fireEvent.pointerDown(widthHandle, { button: 0, pointerId: 7, clientX: 500, clientY: 300, pointerType: "mouse" });
    act(() => {
      window.dispatchEvent(new PointerEvent("pointermove", { pointerId: 7, clientX: 400, clientY: 600 }));
      window.dispatchEvent(new PointerEvent("pointerup", { pointerId: 7, clientX: 400, clientY: 600 }));
    });
    expect(fleetViewStore.get().hostArea).toEqual({ w: 7, h: 1 });
    const decoded = decodeDesignCode(encodeDesignCode(captureSnapshot()));
    expect(decoded.ok && decoded.snapshot.fleet?.hostArea).toEqual({ w: 7, h: 1 });
  });

  it("travels in the design code as presentation, never with the host order", () => {
    setFleetView({ hostArea: { w: 5, h: 4 }, hostOrder: ["delta", "atlas"], placement: "right" });
    const code = encodeDesignCode(captureSnapshot());
    const decoded = decodeDesignCode(code);
    expect(decoded.ok && decoded.snapshot.fleet).toEqual({ mode: "joint", ringSize: 320, placement: "right", fan: true, hostArea: { w: 5, h: 4 } });
    expect(code.includes("delta")).toBe(false);
    fleetViewStore.reset();
    setFleetView({ hostOrder: ["cedar"] });
    if (decoded.ok) applySnapshot(decoded.snapshot);
    expect([fleetViewStore.get().hostArea, fleetViewStore.get().hostOrder]).toEqual([{ w: 5, h: 4 }, ["cedar"]]);
  });
});
