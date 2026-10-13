import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import ContainersPage from "@/features/containers/ContainersPage";
import VolumesPage from "@/features/volumes/VolumesPage";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderRoutes, renderWithApp } from "@/test/render";

beforeEach(() => {
  // These tests cover the one-table overviews; the shipped default shows host blocks.
  preferencesStore.set((p) => ({ ...p, stacksLayout: "table", containersLayout: "table", volumesLayout: "table", imagesLayout: "table" }));
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  vi.restoreAllMocks();
  preferencesStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

/** happy-dom has no layout: a 900 px wide list whose tables always need 1 500 px. */
function narrowLayout() {
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(function (this: HTMLElement) {
    return this.dataset.slot === "overview-fit" ? 900 : 0;
  });
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    return this.tagName === "TABLE" ? new DOMRect(0, 0, 1500, 400) : new DOMRect(0, 0, 0, 0);
  });
}

describe("overview without horizontal scrolling", () => {
  it("falls back to cards that keep every Advanced field, selection and sorting", async () => {
    narrowLayout();
    preferencesStore.set((p) => ({ ...p, containersMode: "advanced" }));
    renderWithApp(<ContainersPage />);
    const list = await screen.findByRole("list", { name: "Container" });
    expect(document.querySelector('[data-slot="overview-fit"]')?.getAttribute("data-view")).toBe("cards");
    expect(document.querySelector("table")).toBe(null);
    const card = within(list).getAllByRole("listitem")[0] as HTMLElement;
    expect(within(card).getAllByRole("term").map((t) => t.textContent)).toEqual(["Zustand", "Host", "Herkunft", "Version", "Update", "Last", "Ports", "Netzwerke", "Neustart", "Läuft seit"]);
    // Selection stays in cards: each card has its checkbox, the bar offers "select all".
    await userEvent.click(within(card).getByRole("checkbox", { name: /auswählen$/ }));
    expect(screen.getByText("1 Container ausgewählt") !== null).toBe(true);
    expect(screen.getByRole("checkbox", { name: "Alle sichtbaren auswählen" }) instanceof HTMLElement).toBe(true);
    // Sorting without headers: key choice and direction.
    expect(screen.getByRole("combobox", { name: "Sortieren nach" }).textContent).toBe("Host");
    await userEvent.click(screen.getByRole("button", { name: "Sortierrichtung umkehren: Aufsteigend" }));
    expect(screen.getByRole("button", { name: "Sortierrichtung umkehren: Absteigend" }) instanceof HTMLButtonElement).toBe(true);
    expect(screen.queryByRole("radiogroup", { name: "Lastwert für Anzeige-Sortierung" }) !== null).toBe(true);
  });

  it("gives volumes the same table/host-block toggle, sortable headers and host headings", async () => {
    renderRoutes([{ path: "/volumes", element: <VolumesPage /> }], ["/volumes"]);
    await screen.findByRole("link", { name: "cloud_data öffnen" });
    const th = (label: string) => screen.getByRole("button", { name: `Nach ${label} sortieren` }).closest("th") as HTMLElement;
    expect(th("Host").getAttribute("aria-sort")).toBe("ascending");
    await userEvent.click(screen.getByRole("button", { name: "Nach Größe sortieren" }));
    expect(th("Größe").getAttribute("aria-sort")).toBe("descending");
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Anordnung" })).getByRole("radio", { name: "Hostblöcke" }));
    expect(preferencesStore.get().volumesLayout).toBe("hosts");
    const blocks = [...screen.getByRole("region", { name: "Volumes nach Host" }).querySelectorAll<HTMLElement>("[data-host-block]")];
    expect(blocks.map((b) => b.dataset.hostBlock)).toEqual(["atlas", "boreal", "cedar", "delta"]);
    expect(within(blocks[0]!).getByRole("heading", { name: "Atlas" }) instanceof HTMLElement).toBe(true);
    // The kind filter still applies inside the blocks.
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Speicherart" })).getByRole("radio", { name: "Bind-Mounts" }));
    expect(screen.queryByRole("link", { name: "cloud_data öffnen" })).toBe(null);
  });
});
