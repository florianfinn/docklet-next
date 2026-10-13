import { fireEvent, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { resetBrandEntrances } from "@/shared/brand/brand-entrance";
import { setUiStyle, uiStyleStore } from "@/shared/preferences/ui-style-store";
import { DEFAULT_VISUAL_STYLE, visualStyleStore } from "@/shared/preferences/visual-style-store";
import { renderWithApp } from "@/test/render";

import { encodeDesignCode, favoriteStore } from "./favorite-code";
import { FavoritesSection } from "./FavoritesSection";
import { VisualLabSection } from "./VisualLabSection";

afterEach(() => {
  visualStyleStore.reset();
  uiStyleStore.reset();
  favoriteStore.reset();
  resetBrandEntrances();
  localStorage.clear();
});

describe("visual lab section", () => {
  it("tunes inside and outside reach separately and switches the logo lockup live", async () => {
    setUiStyle({ pointerGlow: true });
    renderWithApp(<VisualLabSection />);
    await screen.findByText("Reichweite innen");
    // Every handle of the section is reachable by its own name.
    expect(screen.getAllByRole("slider").filter((s) => !s.getAttribute("aria-label"))).toEqual([]);
    const inside = within(document.getElementById("lab-visual-glowInside")!).getByRole("slider", { name: "Reichweite innen" });
    const outside = within(document.getElementById("lab-visual-glowOutside")!).getByRole("slider", { name: "Reichweite außen" });
    fireEvent.keyDown(inside, { key: "ArrowRight" });
    expect([visualStyleStore.get().glowInside, visualStyleStore.get().glowOutside]).toEqual([DEFAULT_VISUAL_STYLE.glowInside + 2, DEFAULT_VISUAL_STYLE.glowOutside]);
    fireEvent.keyDown(outside, { key: "ArrowRight" });
    expect(visualStyleStore.get().glowOutside).toBe(DEFAULT_VISUAL_STYLE.glowOutside + 2);
    fireEvent.click(screen.getByRole("radio", { name: "D im Wort" }));
    expect([visualStyleStore.get().logoVariant, document.querySelector('[data-slot="logo-preview"] svg')?.getAttribute("data-variant")]).toEqual(["initial", "initial"]);
    fireEvent.click(screen.getByRole("switch", { name: /Sanfte 3D-Welle/ }));
    expect(visualStyleStore.get().dotWave).toBe(false);
  });

  it("sets the shared sidebar width and the wave depth, with spoken values", async () => {
    renderWithApp(<VisualLabSection />);
    await screen.findByText("Seitenleiste");
    const width = within(document.getElementById("lab-visual-sidebarWidth")!).getByRole("slider", { name: "Breite" });
    fireEvent.keyDown(width, { key: "ArrowRight" });
    expect([visualStyleStore.get().sidebarWidth, width.getAttribute("aria-valuetext")]).toEqual([DEFAULT_VISUAL_STYLE.sidebarWidth + 0.25, `${DEFAULT_VISUAL_STYLE.sidebarWidth + 0.25} rem`]);
    const depth = within(document.getElementById("lab-visual-dotDepth")!).getByRole("slider", { name: "Tiefe der Welle" });
    fireEvent.keyDown(depth, { key: "Home" });
    expect([visualStyleStore.get().dotDepth, depth.getAttribute("aria-valuetext")]).toEqual([0, "0 %"]);
    // Depth belongs to the wave: with the wave off the slider is disabled.
    fireEvent.click(screen.getByRole("switch", { name: /Sanfte 3D-Welle/ }));
    expect(depth.getAttribute("data-disabled") !== null || depth.getAttribute("aria-disabled") === "true").toBe(true);
  });
});

describe("favourites section", () => {
  it("exports a code, rejects a foreign one with a message and applies a valid one", async () => {
    renderWithApp(<FavoritesSection />);
    fireEvent.click(await screen.findByRole("button", { name: "Aktuellen Code erzeugen" }));
    const code = (document.querySelector('[data-slot="design-code"]') as HTMLTextAreaElement).value;
    expect(code.startsWith("DOCKLET-DESIGN-1:")).toBe(true);

    const field = screen.getByRole("textbox", { name: "Code einfügen" });
    fireEvent.change(field, { target: { value: "https://example.test/?token=abc" } });
    fireEvent.click(screen.getByRole("button", { name: "Code anwenden" }));
    expect([field.getAttribute("aria-invalid"), screen.getByText(/kein Docklet-Design-Code/) !== null]).toEqual(["true", true]);

    fireEvent.change(field, { target: { value: encodeDesignCode({ visual: { ...DEFAULT_VISUAL_STYLE, glowSize: 160 } }) } });
    expect(field.hasAttribute("aria-invalid")).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Code anwenden" }));
    expect(visualStyleStore.get().glowSize).toBe(160);
  });

  it("saves the current design as favourite and shows when it was saved", async () => {
    renderWithApp(<FavoritesSection />);
    const state = () => document.querySelector('[data-slot="favorite-state"]')?.textContent ?? "";
    expect(state()).toMatch(/Noch kein Favorit/);
    fireEvent.click(await screen.findByRole("button", { name: "Aktuelles Design als Favorit speichern" }));
    expect([favoriteStore.get() !== null, /Gespeichert/.test(state())]).toEqual([true, true]);
    const clear = screen.getByRole("button", { name: "Favorit löschen" });
    fireEvent.click(clear);
    expect([favoriteStore.get(), within(document.body).getByRole("button", { name: "Favorit löschen" }).hasAttribute("disabled")]).toEqual([null, true]);
  });
});
