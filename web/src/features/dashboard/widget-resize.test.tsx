import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";

import DashboardPage from "./DashboardPage";
import { DEFAULT_LAYOUT, layoutStore, mergeLayout, widgetSize } from "./dashboard-layout";
import { columnsFor, snapSize } from "./use-widget-grid";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  // Every core widget at its default size, independent of the factory layout.
  layoutStore.set({ ...DEFAULT_LAYOUT, hidden: [], sizes: {} });
});

afterEach(() => {
  vi.restoreAllMocks();
  layoutStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

const widget = (id: string) =>
  waitFor(() => {
    const el = document.querySelector<HTMLElement>(`[data-widget="${id}"]`);
    if (!el) throw new Error(`widget ${id} missing`);
    return el;
  });
const stored = () => JSON.parse(localStorage.getItem("docklet-react-opus.dashboard") ?? "{}").data?.sizes;

describe("widget grid model", () => {
  it("derives columns from the width and snaps pointer deltas to whole cells within limits", () => {
    // 22 rem columns with 1 rem gaps at 16 px: phone 1, laptop 2, full HD 4, 21:9 8.
    expect([358, 830, 1544, 3088].map((w) => columnsFor(w))).toEqual([1, 2, 4, 8]);
    const cell = { col: 384, row: 80, gap: 16 };
    const start = { w: 1, h: 4 };
    const px = { w: 384, h: 4 * 80 + 3 * 16 };
    expect(snapSize("load", start, px, { x: 380, y: 0 }, cell, 4)).toEqual({ w: 2, h: 4 });
    expect(snapSize("load", start, px, { x: 150, y: 210 }, cell, 4)).toEqual({ w: 1, h: 6 });
    // Never below the widget's minimum, never wider than the grid or the maximum.
    expect(snapSize("updates", start, px, { x: -900, y: -900 }, cell, 4)).toEqual({ w: 1, h: 4 });
    expect(snapSize("load", start, px, { x: 5000, y: 5000 }, cell, 3)).toEqual({ w: 3, h: 10 });
  });

  it("keeps only valid stored sizes", () => {
    const merged = mergeLayout({ ...DEFAULT_LAYOUT, sizes: { attention: { w: 9, h: 40 }, load: { w: 0, h: 1 }, bogus: { w: 2, h: 2 }, timeline: { w: "x" } } }, DEFAULT_LAYOUT);
    expect(merged.sizes).toEqual({ attention: { w: 4, h: 10 }, load: { w: 1, h: 3 }, timeline: { w: 1, h: 4 } });
    expect(widgetSize(merged, "capacity")).toEqual({ w: 1, h: 4 });
  });
});

describe("widget resizing in edit mode", () => {
  it("resizes by keyboard and buttons, persists per widget and resets", async () => {
    renderWithApp(<DashboardPage />);
    const card = await widget("load");
    await userEvent.click(screen.getByRole("button", { name: "Anpassen" }));
    card.focus();
    fireEvent.keyDown(card, { key: "ArrowDown", altKey: true, shiftKey: true });
    fireEvent.keyDown(card, { key: "ArrowDown", altKey: true, shiftKey: true });
    expect(stored()).toEqual({ load: { w: 1, h: 6 } });
    expect(document.querySelector('[data-widget-slot="load"]')?.getAttribute("data-size")).toBe("1x6");
    expect((document.querySelector('[data-widget-slot="load"]') as HTMLElement).style.gridRow).toBe("span 6");
    expect(screen.getByText("Lastverlauf: 1 Spalte × 6 Reihen") !== null).toBe(true);
    // Shrinking stops at the widget's minimum of three rows.
    for (let i = 0; i < 6; i++) fireEvent.keyDown(card, { key: "ArrowUp", altKey: true, shiftKey: true });
    expect(stored()).toEqual({ load: { w: 1, h: 3 } });

    await userEvent.click(within(card).getByRole("button", { name: "Größe von Lastverlauf: 1 × 3" }));
    const popup = await screen.findByRole("dialog", { name: "Größe von Lastverlauf" });
    // One column in this test environment: wider is not possible, lower stops at the minimum.
    expect([(within(popup).getByRole("button", { name: "Breiter" }) as HTMLButtonElement).disabled, (within(popup).getByRole("button", { name: "Niedriger" }) as HTMLButtonElement).disabled]).toEqual([true, true]);
    await userEvent.click(within(popup).getByRole("button", { name: "Höher" }));
    // Back at the default size: nothing is stored for this widget.
    expect([stored(), widgetSize(layoutStore.get(), "load")]).toEqual([{}, { w: 1, h: 4 }]);
    await userEvent.click(within(popup).getByRole("button", { name: "Höher" }));
    await userEvent.click(within(popup).getByRole("button", { name: "Standardgröße" }));
    expect(layoutStore.get().sizes).toEqual({});
  });

  it("resizes with the corner handle in whole rows without starting a move; Escape cancels", async () => {
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      return this.dataset.widgetSlot ? new DOMRect(0, 0, 384, 368) : new DOMRect(0, 0, 0, 0);
    });
    renderWithApp(<DashboardPage />);
    await widget("timeline");
    await userEvent.click(screen.getByRole("button", { name: "Anpassen" }));
    const handle = screen.getByRole("button", { name: "Größe von Betriebsverlauf 24 h ändern (ziehen oder Pfeiltasten)" });

    fireEvent.pointerDown(handle, { button: 0, pointerId: 7, clientX: 380, clientY: 360, pointerType: "mouse" });
    act(() => {
      window.dispatchEvent(new PointerEvent("pointermove", { pointerId: 7, clientX: 380, clientY: 570 }));
    });
    // Snapped preview while dragging; no drag-to-move placeholder.
    expect(document.querySelector('[data-widget-slot="timeline"] [data-slot="resize-preview"]')?.textContent).toBe("1 Spalte × 6 Reihen");
    expect(document.querySelector('[data-slot="drop-placeholder"]')).toBe(null);
    act(() => {
      window.dispatchEvent(new PointerEvent("pointerup", { pointerId: 7, clientX: 380, clientY: 570 }));
    });
    expect(widgetSize(layoutStore.get(), "timeline")).toEqual({ w: 1, h: 6 });

    fireEvent.pointerDown(handle, { button: 0, pointerId: 8, clientX: 380, clientY: 360, pointerType: "mouse" });
    act(() => {
      window.dispatchEvent(new PointerEvent("pointermove", { pointerId: 8, clientX: 380, clientY: 800 }));
    });
    act(() => {
      window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    });
    expect(widgetSize(layoutStore.get(), "timeline")).toEqual({ w: 1, h: 6 });
    expect(document.querySelector('[data-slot="resize-preview"]')).toBe(null);
  });
});
