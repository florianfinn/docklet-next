import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";

import DashboardPage from "./DashboardPage";
import { DEFAULT_LAYOUT, layoutStore, mergeLayout, reorderVisible, visibleWidgets } from "./dashboard-layout";
import { insertAt } from "./use-sortable-grid";

const W = 400;
const H = 380;

/** Lays slots out in a 3-column grid by DOM order, since happy-dom has no layout engine. */
function mockGridRects() {
  return vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    const slot = this.closest<HTMLElement>("[data-widget-slot]");
    if (!slot) return new DOMRect(0, 0, 0, 0);
    const i = [...(slot.parentElement?.children ?? [])].indexOf(slot);
    return new DOMRect((i % 3) * W, Math.floor(i / 3) * H, W - 10, H - 10);
  });
}

const widget = (id: string) =>
  waitFor(() => {
    const el = document.querySelector<HTMLElement>(`[data-widget="${id}"]`);
    if (!el) throw new Error(`widget ${id} missing`);
    return el;
  });

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  vi.restoreAllMocks();
  layoutStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

describe("dashboard layout model", () => {
  it("moves a dragged id before or after a target and keeps hidden widgets in place", () => {
    expect(insertAt(["a", "b", "c", "d"], "a", "c", true)).toEqual(["b", "c", "a", "d"]);
    expect(insertAt(["a", "b", "c", "d"], "d", "b", false)).toEqual(["a", "d", "b", "c"]);
    const layout = { ...DEFAULT_LAYOUT, order: [...DEFAULT_LAYOUT.order] };
    const visible = visibleWidgets(layout);
    const next = reorderVisible(layout, [visible[1]!, visible[0]!, ...visible.slice(2)]);
    expect(visibleWidgets(next).slice(0, 2)).toEqual([visible[1], visible[0]]);
    expect(next.order.indexOf("updates")).toBe(layout.order.indexOf("updates"));
  });

  it("never restores secrets from storage and appends widgets added later", () => {
    const merged = mergeLayout({ order: ["timeline", "attention"], hidden: [], services: { plex: { address: "http://192.0.2.21:32400", status: "connected", token: "x-secret" } } }, DEFAULT_LAYOUT);
    expect(JSON.stringify(merged).includes("x-secret")).toBe(false);
    expect(merged.services.plex).toEqual({ address: "http://192.0.2.21:32400", status: "connected", simulateError: false });
    expect(merged.order.slice(0, 2)).toEqual(["timeline", "attention"]);
    expect(merged.hidden.includes("minecraft") && merged.order.includes("overseerr")).toBe(true);
  });
});

describe("dashboard edit mode", () => {
  it("reorders widgets by pointer drag with a placeholder and by keyboard", async () => {
    mockGridRects();
    renderWithApp(<DashboardPage />);
    await widget("attention");
    await userEvent.click(screen.getByRole("button", { name: "Anpassen" }));
    const before = visibleWidgets(layoutStore.get());
    const card = document.querySelector<HTMLElement>(`[data-widget="${before[0]}"]`) as HTMLElement;

    fireEvent.pointerDown(card, { button: 0, pointerId: 1, clientX: 50, clientY: 50, pointerType: "mouse" });
    act(() => {
      window.dispatchEvent(new PointerEvent("pointermove", { pointerId: 1, clientX: 70, clientY: 60 }));
    });
    // Dragging: the slot shows a drop placeholder and the card follows the pointer by transform.
    expect(document.querySelector(`[data-widget-slot="${before[0]}"] [data-slot="drop-placeholder"]`) !== null).toBe(true);
    expect(card.style.transform.startsWith("translate(")).toBe(true);
    // Over the right half of the third card: insert after it.
    act(() => {
      window.dispatchEvent(new PointerEvent("pointermove", { pointerId: 1, clientX: 2 * W + 300, clientY: 100 }));
    });
    act(() => {
      window.dispatchEvent(new PointerEvent("pointerup", { pointerId: 1, clientX: 2 * W + 300, clientY: 100 }));
    });
    await waitFor(() => expect(visibleWidgets(layoutStore.get()).slice(0, 3)).toEqual([before[1], before[2], before[0]]));
    expect(JSON.parse(localStorage.getItem("docklet-react-opus.dashboard") ?? "{}").data.order.indexOf(before[0])).toBeGreaterThan(1);

    // Keyboard alternative with an announcement.
    const moved = document.querySelector<HTMLElement>(`[data-widget="${before[0]}"]`) as HTMLElement;
    moved.focus();
    fireEvent.keyDown(moved, { key: "ArrowLeft", altKey: true });
    expect(visibleWidgets(layoutStore.get()).indexOf(before[0]!)).toBe(1);
    expect(screen.getByText(/an Position 2 von/) !== null).toBe(true);
  });

  it("adds a service widget from the catalog and connects it in a simulation without storing secrets", async () => {
    layoutStore.set((l) => ({ ...l, hidden: [...new Set([...l.hidden, "plex", "minecraft"] as const)] }));
    renderWithApp(<DashboardPage />);
    await widget("attention");
    await userEvent.click(screen.getByRole("button", { name: "Anpassen" }));
    await userEvent.click(screen.getByRole("button", { name: "Widget hinzufügen" }));
    const catalog = await screen.findByRole("dialog", { name: "Widget-Katalog" });
    await userEvent.click(within(catalog).getByRole("button", { name: "Plex einblenden" }));
    await userEvent.click(within(catalog).getByRole("button", { name: "Minecraft einblenden" }));
    await userEvent.keyboard("{Escape}");

    // Without an address the widget says the configuration is missing.
    const plex = await widget("plex");
    expect(within(plex).getByText("Konfiguration fehlt") !== null).toBe(true);
    await userEvent.click(within(plex).getByRole("button", { name: "Plex einrichten" }));
    const dialog = await screen.findByRole("dialog", { name: "Plex einrichten" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Beispiel" }));
    await userEvent.type(within(dialog).getByLabelText("Plex-Token"), "geheimer-token");
    await userEvent.click(within(dialog).getByRole("button", { name: "Verbinden (simuliert)" }));
    await waitFor(() => expect(within(plex).getByRole("list", { name: "Aktive Streams" }) !== null).toBe(true), { timeout: 3000 });
    expect(within(plex).getAllByRole("progressbar").length).toBe(3);
    expect(localStorage.getItem("docklet-react-opus.dashboard")?.includes("geheimer-token")).toBe(false);

    // The error simulation ends in the error state of the widget.
    const minecraft = await widget("minecraft");
    await userEvent.click(within(minecraft).getByRole("button", { name: "Minecraft einrichten" }));
    const mc = await screen.findByRole("dialog", { name: "Minecraft einrichten" });
    await userEvent.type(within(mc).getByLabelText("Adresse"), "192.0.2.22:25565");
    await userEvent.click(within(mc).getByLabelText("Fehler beim Verbinden simulieren"));
    await userEvent.click(within(mc).getByRole("button", { name: "Verbinden (simuliert)" }));
    await waitFor(() => expect(within(minecraft).getByText("Verbindung fehlgeschlagen") !== null).toBe(true), { timeout: 3000 });
  }, 20_000);
});
