import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderWithApp } from "@/test/render";

import { cappedGrid } from "@/shared/lib/host-block-grid";

import StacksPage from "./StacksPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  preferencesStore.set((p) => ({ ...p, stacksLayout: "hosts" }));
});

afterEach(() => {
  vi.restoreAllMocks();
  preferencesStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

const atlasNames = () => within(screen.getByRole("region", { name: /Atlas/ })).getAllByRole("link", { name: /^Stack .+ öffnen$/ }).map((a) => a.textContent);

describe("stack host blocks", () => {
  it("sort through their table headers without a sort bar above the blocks", async () => {
    renderWithApp(<StacksPage />);
    await screen.findByRole("link", { name: "Stack Media öffnen" });
    const blocks = document.querySelector<HTMLElement>('[data-slot="host-blocks"]')!;
    expect([blocks.querySelector('[data-slot="card-sort-bar"]') === null, blocks.querySelector('[data-slot="host-block-sort"]') === null]).toEqual([true, true]);

    const atlas = screen.getByRole("region", { name: /Atlas/ });
    expect(atlasNames()).toEqual(["Home", "Media", "Paperless"]);
    await userEvent.click(within(atlas).getByRole("button", { name: "Nach Stack sortieren" }));
    await userEvent.click(within(atlas).getByRole("button", { name: "Nach Stack sortieren" }));
    // Header sorting applies to every block.
    expect(atlasNames()).toEqual(["Paperless", "Media", "Home"]);
    expect(within(atlas).getByRole("button", { name: "Nach Stack sortieren" }).closest("th")?.getAttribute("aria-sort")).toBe("descending");
  });

  it("offer a compact sort control in the header of blocks that fell back to cards", async () => {
    // Every block's table overflows its column, so all blocks fall back to cards.
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(300);
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({ width: 900, height: 40, top: 0, left: 0, right: 900, bottom: 40, x: 0, y: 0, toJSON: () => ({}) });
    renderWithApp(<StacksPage />);
    await screen.findByRole("link", { name: "Stack Media öffnen" });
    const blocks = document.querySelector<HTMLElement>('[data-slot="host-blocks"]')!;
    const atlas = screen.getByRole("region", { name: /Atlas/ });
    expect([atlas.getAttribute("data-view"), blocks.querySelector(':scope > div > [data-slot="card-sort-bar"]') === null]).toEqual(["cards", true]);

    const header = within(atlas.querySelector<HTMLElement>("header")!);
    expect(atlasNames()).toEqual(["Home", "Media", "Paperless"]);
    // Radix Select opens from the keyboard under happy-dom.
    header.getByRole("combobox", { name: "Sortieren nach (Atlas)" }).focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.click(await screen.findByRole("option", { name: "Stack" }));
    await userEvent.click(within(screen.getByRole("region", { name: /Atlas/ }).querySelector<HTMLElement>("header")!).getByRole("button", { name: "Sortierrichtung umkehren: Aufsteigend (Atlas)" }));
    expect(atlasNames()).toEqual(["Paperless", "Media", "Home"]);
    expect(within(screen.getByRole("region", { name: /Atlas/ }).querySelector<HTMLElement>("header")!).getByRole("button", { name: "Sortierrichtung umkehren: Absteigend (Atlas)" }) instanceof HTMLElement).toBe(true);
  });

  it("cap compact at three and advanced at two blocks per row and bound a lone host's width", async () => {
    renderWithApp(<StacksPage />);
    await screen.findByRole("link", { name: "Stack Media öffnen" });
    const grid = () => document.querySelector<HTMLElement>('[data-slot="host-block-grid"]')!;
    expect([grid().dataset.maxColumns, grid().style.maxWidth]).toEqual(["3", "calc(3 * 56rem + 2rem)"]);
    await userEvent.click(screen.getByRole("radio", { name: /Advanced/ }));
    expect([grid().dataset.maxColumns, grid().style.maxWidth]).toEqual(["2", "calc(2 * 84rem + 1rem)"]);
    await userEvent.click(screen.getByRole("radio", { name: /Compact/ }));
    // A search leaves one host block: the grid is one bounded block wide, not the whole row.
    await userEvent.type(screen.getByRole("searchbox", { name: /Stacks/ }), "Media");
    expect([document.querySelectorAll("[data-host-block]").length, grid().style.maxWidth]).toEqual([1, "calc(1 * 56rem + 0rem)"]);
  });

  it("derive the column floor from the cap so no further column fits", () => {
    const style = cappedGrid(3, 5, "56rem");
    expect([style.gridTemplateColumns, style.maxWidth]).toEqual(["repeat(auto-fit, minmax(min(100%, max(var(--hb-min), calc((100% - 2rem) / 3))), 1fr))", "calc(3 * 56rem + 2rem)"]);
    expect(cappedGrid(2, 1).maxWidth).toBe(undefined);
  });
});
