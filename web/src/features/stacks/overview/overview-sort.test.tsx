import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import ContainersPage from "@/features/containers/ContainersPage";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderWithApp } from "@/test/render";

import StacksPage from "./StacksPage";

beforeEach(() => {
  // These tests cover the one-table overviews; the shipped default shows host blocks.
  preferencesStore.set((p) => ({ ...p, stacksLayout: "table", containersLayout: "table", volumesLayout: "table", imagesLayout: "table" }));
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  preferencesStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

const stackNames = () => screen.getAllByRole("link", { name: /^Stack .+ öffnen$/ }).map((a) => a.textContent);
const header = (name: string) => screen.getByRole("button", { name: `Nach ${name} sortieren` }).closest("th") as HTMLElement;

describe("stack overview sorting", () => {
  it("sorts by clickable headers with aria-sort, host then name by default", async () => {
    renderWithApp(<StacksPage />);
    await screen.findByRole("link", { name: "Stack Media öffnen" });
    expect(screen.queryByRole("combobox", { name: "Sortierung" }) === null).toBe(true);
    expect(header("Host").getAttribute("aria-sort")).toBe("ascending");
    // Atlas: Home, Media, Paperless · Boreal: Cloud, Craft, Monitoring · Cedar: Wiki · Delta: DNS, Git
    expect(stackNames()).toEqual(["Home", "Media", "Paperless", "Cloud", "Craft", "Monitoring", "Wiki", "DNS", "Git"]);

    await userEvent.click(screen.getByRole("button", { name: "Nach Host sortieren" }));
    expect(header("Host").getAttribute("aria-sort")).toBe("descending");
    // Hosts reversed, names inside each host still A–Z.
    expect(stackNames()).toEqual(["DNS", "Git", "Wiki", "Cloud", "Craft", "Monitoring", "Home", "Media", "Paperless"]);

    await userEvent.click(screen.getByRole("button", { name: "Nach Stack sortieren" }));
    expect([header("Stack").getAttribute("aria-sort"), header("Host").getAttribute("aria-sort")]).toEqual(["ascending", "none"]);
    expect(stackNames()).toEqual(["Cloud", "Craft", "DNS", "Git", "Home", "Media", "Monitoring", "Paperless", "Wiki"]);
    expect(header("Stack").hasAttribute("data-sorted")).toBe(true);
  });

  it("shows host blocks with the same filters, sort and cross-block selection, and persists the layout", async () => {
    preferencesStore.set((p) => ({ ...p, stacksMode: "advanced" }));
    renderWithApp(<StacksPage />);
    await screen.findByRole("link", { name: "Stack Media öffnen" });
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Anordnung" })).getByRole("radio", { name: "Hostblöcke" }));
    expect(preferencesStore.get().stacksLayout).toBe("hosts");
    const region = screen.getByRole("region", { name: "Stacks nach Host" });
    const blocks = [...region.querySelectorAll<HTMLElement>("[data-host-block]")];
    expect(blocks.map((b) => b.dataset.hostBlock)).toEqual(["atlas", "boreal", "cedar", "delta"]);
    expect(within(blocks[0] as HTMLElement).getByText("3 Stacks · 3 laufen") !== null).toBe(true);
    // Rows still open the stack directly; no expand control inside a block.
    expect(within(blocks[0] as HTMLElement).getByRole("link", { name: "Stack Media öffnen" }).getAttribute("href")).toBe("/stacks/media");
    expect(within(blocks[0] as HTMLElement).queryByRole("button", { name: /aufklappen/i }) === null).toBe(true);

    await userEvent.click(within(blocks[0] as HTMLElement).getByRole("checkbox", { name: "Alle Stacks auf Atlas auswählen" }));
    await userEvent.click(within(blocks[1] as HTMLElement).getByRole("checkbox", { name: "Cloud auswählen" }));
    expect(screen.getByRole("toolbar", { name: "4 Stacks ausgewählt" }) !== null).toBe(true);

    // Sorting inside blocks follows the same header state; the host order button flips the blocks.
    await userEvent.click(within(blocks[0] as HTMLElement).getByRole("button", { name: "Nach Stack sortieren" }));
    await userEvent.click(within(blocks[0] as HTMLElement).getByRole("button", { name: "Nach Stack sortieren" }));
    expect(within(blocks[0] as HTMLElement).getAllByRole("link", { name: /^Stack .+ öffnen$/ }).map((a) => a.textContent)).toEqual(["Paperless", "Media", "Home"]);
    await userEvent.click(screen.getByRole("button", { name: /^Hostreihenfolge/ }));
    expect([...region.querySelectorAll<HTMLElement>("[data-host-block]")].map((b) => b.dataset.hostBlock)).toEqual(["delta", "cedar", "boreal", "atlas"]);

    // A search hides blocks without matches.
    await userEvent.type(screen.getByRole("searchbox", { name: "Stacks durchsuchen" }), "jellyfin");
    expect([...region.querySelectorAll<HTMLElement>("[data-host-block]")].map((b) => b.dataset.hostBlock)).toEqual(["atlas"]);
  });

  it("sorts containers by header too and offers host blocks", async () => {
    renderWithApp(<ContainersPage />);
    const first = await screen.findAllByRole("button", { name: /öffnen$/ });
    expect(first.length > 10).toBe(true);
    expect(header("Host").getAttribute("aria-sort")).toBe("ascending");
    const names = () => screen.getAllByRole("button", { name: /^Details zu .+ öffnen$|öffnen$/ }).map((b) => b.textContent);
    expect(names().slice(0, 3)).toEqual(["duplicati", "home-homeassistant-1", "home-mosquitto-1"]);
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Anordnung" })).getByRole("radio", { name: "Hostblöcke" }));
    expect(screen.getByRole("region", { name: "Container nach Host" }).querySelectorAll("[data-host-block]").length).toBe(4);
    expect(screen.queryByText("docklet-agent") === null).toBe(true);
  });
});
