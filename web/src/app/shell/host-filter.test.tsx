import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { ControlsSection } from "@/features/design-lab/ControlsSection";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { setUiStyle, uiStyleStore } from "@/shared/preferences/ui-style-store";
import { SidebarProvider } from "@/shared/ui/sidebar";
import { renderWithApp } from "@/test/render";

import { HostFilter } from "./HostFilter";

beforeEach(() => {
  // Starts from the list design; the shipped default is the menu.
  setUiStyle({ hostFilterStyle: "list" });
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  preferencesStore.reset();
  uiStyleStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

const hostButtons = () => screen.getAllByRole("button", { name: /^(Atlas|Boreal|Cedar|Delta), / });

describe("host filter designs", () => {
  it("list, chips and menu edit the same selection in alphabetical order", async () => {
    renderWithApp(
      <SidebarProvider>
        <HostFilter />
        <ControlsSection />
      </SidebarProvider>,
    );
    await screen.findByRole("button", { name: /^Atlas, Online/ });
    expect(hostButtons().map((b) => b.getAttribute("aria-label")?.split(",")[0])).toEqual(["Atlas", "Boreal", "Cedar", "Delta"]);
    // A disconnected host says so in words, not only by colour.
    expect(screen.getByRole("button", { name: /^Cedar, Getrennt/ }).textContent?.includes("Getrennt")).toBe(true);

    await userEvent.click(screen.getByRole("button", { name: /^Cedar, Getrennt/ }));
    expect(preferencesStore.get().hostSelection).toEqual(["atlas", "boreal", "delta"]);
    // Host names are the toggles themselves: no checkbox, no check mark, selection in aria-pressed.
    const noCheckbox = () => document.querySelectorAll('[role="checkbox"], [data-host-toggle] .lucide-check').length === 0 && document.querySelectorAll("[data-host-toggle]").length > 0;
    expect(noCheckbox()).toBe(true);
    expect(hostButtons().map((b) => `${b.getAttribute("aria-pressed")}/${b.hasAttribute("data-on")}`)).toEqual(["true/true", "true/true", "false/false", "true/true"]);
    // Hidden is not the same as disconnected: Cedar is both and still names its status.
    expect(screen.getByRole("button", { name: /^Cedar, Getrennt, ausgeblendet/ }).textContent?.includes("Getrennt")).toBe(true);

    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Hostfilter-Design" })).getByRole("radio", { name: /Chips/ }));
    expect(uiStyleStore.get().hostFilterStyle).toBe("chips");
    expect(document.querySelector('[data-host-filter="chips"]') !== null).toBe(true);
    expect(screen.getByRole("button", { name: /^Cedar, Getrennt, ausgeblendet/ }).getAttribute("aria-pressed")).toBe("false");
    expect(noCheckbox()).toBe(true);

    act(() => setUiStyle({ hostFilterStyle: "menu" }));
    // The first trigger is the menu design; the second belongs to the collapsed icon sidebar.
    await userEvent.click(screen.getAllByRole("button", { name: "Hostfilter: 3 von 4 Hosts" })[0] as HTMLElement);
    const online = await screen.findByRole("group", { name: "Hostfilter: Online" });
    const offline = screen.getByRole("group", { name: "Hostfilter: Getrennt" });
    expect(within(online).getAllByRole("button").map((b) => b.textContent)).toEqual(["Atlas", "Boreal", "Delta"]);
    expect(noCheckbox()).toBe(true);
    await userEvent.click(within(offline).getByRole("button", { name: /^Cedar/ }));
    expect(preferencesStore.get().hostSelection).toEqual([]);
    expect(JSON.parse(localStorage.getItem("docklet-react-opus.ui-style") ?? "{}").data.hostFilterStyle).toBe("menu");
  });
});
